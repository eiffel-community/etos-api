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
package v1alpha1

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/eiffel-community/eiffelevents-sdk-go"
	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/eiffel-community/etos-api/internal/eventrepository"

	"github.com/sethvargo/go-retry"
	"github.com/sirupsen/logrus"
)

type ArtifactResult struct {
	Artifact eiffelevents.ArtifactCreatedV3
	err      error
}

// waitForArtifact waits for an artifact created and reports it to a channel.
func waitForArtifact(ctx context.Context, cfg config.Config, logger *logrus.Entry, request StartRequest, channel chan ArtifactResult) {
	artifact, err := artifactCreated(ctx, cfg, logger, request)
	channel <- ArtifactResult{
		Artifact: artifact,
		err:      err,
	}
}

// artifactCreated waits for an artifact created event in the event repository.
func artifactCreated(ctx context.Context, cfg config.Config, logger *logrus.Entry, request StartRequest) (eiffelevents.ArtifactCreatedV3, error) {
	var artifact eiffelevents.ArtifactCreatedV3
	er := eventrepository.NewGraphQLAPI(cfg, logger)
	logger.Infof("Getting artifact (%v) from event repository", request)
	err := retry.Fibonacci(ctx, 1*time.Second, func(ctx context.Context) error {
		var response eiffelevents.ArtifactCreatedV3
		var err error
		if request.ArtifactID != "" {
			response, err = er.GetArtifactByID(ctx, request.ArtifactID)
		} else if request.ArtifactIdentity != "" {
			response, err = er.GetArtifactByIdentity(ctx, request.ArtifactIdentity)
		} else {
			return NewHTTPError(errors.New("at least one of 'artifact_identity' or 'articact_id' is required"), http.StatusBadRequest)
		}
		switch err.(type) {
		case nil:
			artifact = response
			return nil
		case *eventrepository.NoArtifact:
			return NewHTTPError(err, http.StatusBadRequest)
		default:
			return retry.RetryableError(err)
		}
	})
	if err != nil {
		logger.Errorf("failed getting artifact: %+v", err)
		// Yes, it's correct with double '&' in these two statements.
		// This is because 'As panics if target is not a non-nil pointer to
		// either a type that implements error, or to any interface type'.
		errType := &HTTPError{}
		if errors.As(err, &errType) {
			return artifact, err
		}
		return artifact, NewHTTPError(err, http.StatusRequestTimeout)
	}
	logger.Infof("artifact received: %s (%s)", artifact.Data.Identity, artifact.Meta.ID)
	return artifact, err
}
