// Copyright 2021 Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package eventrepository

import (
	"context"
	"fmt"

	"github.com/eiffel-community/etos-api/internal/config"

	"github.com/eiffel-community/eiffelevents-sdk-go"
	"github.com/machinebox/graphql"
	"github.com/sirupsen/logrus"
)

// EventRepository is the interface used no matter which event repository is used.
// Only EiffelGraphQLAPI is supported today, but we might want to support others in
// the future.
type EventRepository interface {
	GetArtifactByID(context.Context, string) (eiffelevents.ArtifactCreatedV3, error)
	GetArtifactByIdentity(context.Context, string) (eiffelevents.ArtifactCreatedV3, error)
}

// GraphQLAPI implements EventRepository
type GraphQLAPI struct {
	cfg    config.Config
	logger *logrus.Entry
	client *graphql.Client
}

// NewGraphQLAPI creates a new GraphQL client implementing the EventRepository interface.
func NewGraphQLAPI(cfg config.Config, logger *logrus.Entry) EventRepository {
	return &GraphQLAPI{
		cfg:    cfg,
		logger: logger,
		client: graphql.NewClient(cfg.EventRepositoryHost()),
	}
}

// GetArtifactByIdentity uses the 'meta.id' key in artifact created events to find
// the artifact representation in ER.
func (er *GraphQLAPI) GetArtifactByID(ctx context.Context, ID string) (eiffelevents.ArtifactCreatedV3, error) {
	return er.artifact(ctx, fmt.Sprintf("{'meta.id': '%s'}", ID))
}

// GetArtifactByIdentity uses the 'data.identity' key in artifact created events to find
// the artifact representation in ER.
func (er *GraphQLAPI) GetArtifactByIdentity(ctx context.Context, identity string) (eiffelevents.ArtifactCreatedV3, error) {
	return er.artifact(ctx, fmt.Sprintf("{'data.identity': {'$regex': '%s'}}", identity))
}

// ArtifactQuery is a generic query for finding artifact created identity and id.
// It requires a searchString to be added as a parameter.
var ArtifactQuery = `
query ArtifactQuery($searchString: String) {
	artifactCreated(search: $searchString, last: 1) {
		edges {
			node {
				data {
					identity
				}
				meta {
					id
				}
			}
		}
	}
}
`

// Edge holds the Artifact node.
type Edge struct {
	Node eiffelevents.ArtifactCreatedV3 `json:"node"`
}

// GraphQLArtifact is a slice of Edges that represent artifacts.
type GraphQLArtifact struct {
	Edges []Edge `json:"edges"`
}

// GraphQLResponse is the response from the GraphQL event repository.
type GraphQLResponse struct {
	ArtifactCreated GraphQLArtifact `json:"artifactCreated"`
}

// NoArtifact indicates that no artifacts were found.
type NoArtifact struct {
	Message string
}

// Error returns the string representation of the NoArtifact error.
func (e *NoArtifact) Error() string {
	return e.Message
}

// artifact makes a request against the event repository with the searchString provided.
func (er *GraphQLAPI) artifact(ctx context.Context, searchString string) (eiffelevents.ArtifactCreatedV3, error) {
	request := graphql.NewRequest(ArtifactQuery)
	request.Var("searchString", searchString)

	var response GraphQLResponse
	if err := er.client.Run(ctx, request, &response); err != nil {
		return eiffelevents.ArtifactCreatedV3{}, err
	}
	edges := response.ArtifactCreated.Edges
	if len(edges) == 0 {
		return eiffelevents.ArtifactCreatedV3{}, &NoArtifact{fmt.Sprintf("no artifact returned for %s", searchString)}
	}
	// The query is limited to, at most, one response and we test that it is not zero just before this
	// which makes it safe to get the 0 index from edges.
	return edges[0].Node, nil

}
