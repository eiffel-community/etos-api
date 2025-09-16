// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package kubernetes

import (
	"context"
	"fmt"
	"time"

	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// extractCacheEntryKey extracts a cached object from the cache by key
func extractCacheEntryKey(obj interface{}) (string, error) {
	if entry, ok := obj.(cacheEntry); ok {
		return entry.key, nil
	}
	return "", fmt.Errorf("invalid cache object type")
}

// cacheEntry contains the cached data with its key
type cacheEntry struct {
	key  string
	data interface{}
}

// kubernetesCache wraps TTL stores for different resource types
type kubernetesCache struct {
	jobsStore cache.Store
	podsStore cache.Store
	cacheTTL  time.Duration
}

// newKubernetesCache creates a new cache with TTL stores
func newKubernetesCache() *kubernetesCache {
	ttl := 5 * time.Second
	return &kubernetesCache{
		jobsStore: cache.NewTTLStore(extractCacheEntryKey, ttl),
		podsStore: cache.NewTTLStore(extractCacheEntryKey, ttl),
		cacheTTL:  ttl,
	}
}

// getAllJobs retrieves all jobs from cache or API, using TTL store for automatic expiration
func (c *kubernetesCache) getAllJobs(ctx context.Context, client *kubernetes.Clientset, namespace string, logger *logrus.Entry) (*v1.JobList, error) {
	// Use namespace as cache key since we're caching all jobs in the namespace
	key := fmt.Sprintf("all_jobs_%s", namespace)

	// Try to get from cache first
	if cached, exists, err := c.jobsStore.GetByKey(key); err == nil && exists {
		if entry, ok := cached.(cacheEntry); ok {
			if jobs, ok := entry.data.(*v1.JobList); ok {
				logger.Infof("Returning cached jobs for namespace: %s (count: %d)", namespace, len(jobs.Items))
				return jobs, nil
			}
		}
	}

	// Fetch from API if not in cache
	logger.Infof("Making Kubernetes API call to fetch all jobs for namespace: %s", namespace)
	jobs, err := client.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Errorf("Failed to fetch jobs from Kubernetes API for namespace %s: %v", namespace, err)
		return nil, err
	}

	// Store in cache
	c.jobsStore.Add(cacheEntry{
		key:  key,
		data: jobs,
	})
	logger.Infof("Successfully fetched and cached %d jobs for namespace: %s", len(jobs.Items), namespace)
	return jobs, nil
}

// getAllPods retrieves all pods from cache or API, using TTL store for automatic expiration
func (c *kubernetesCache) getAllPods(ctx context.Context, client *kubernetes.Clientset, namespace string, logger *logrus.Entry) (*corev1.PodList, error) {
	// Use namespace as cache key since we're caching all pods in the namespace
	key := fmt.Sprintf("all_pods_%s", namespace)

	// Try to get from cache first
	if cached, exists, err := c.podsStore.GetByKey(key); err == nil && exists {
		if entry, ok := cached.(cacheEntry); ok {
			if pods, ok := entry.data.(*corev1.PodList); ok {
				logger.Infof("Returning cached pods for namespace: %s (count: %d)", namespace, len(pods.Items))
				return pods, nil
			}
		}
	}

	// Fetch from API if not in cache
	logger.Infof("Making Kubernetes API call to fetch all pods for namespace: %s", namespace)
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Errorf("Failed to fetch pods from Kubernetes API for namespace %s: %v", namespace, err)
		return nil, err
	}

	// Store in cache
	c.podsStore.Add(cacheEntry{
		key:  key,
		data: pods,
	})

	logger.Infof("Successfully fetched and cached %d pods for namespace: %s", len(pods.Items), namespace)
	return pods, nil
}

type Kubernetes struct {
	logger    *logrus.Entry
	config    *rest.Config
	client    *kubernetes.Clientset
	namespace string
	cache     *kubernetesCache
}

// New creates a new Kubernetes struct.
func New(cfg config.Config, log *logrus.Entry) *Kubernetes {
	return &Kubernetes{
		logger:    log,
		namespace: cfg.ETOSNamespace(),
		cache:     newKubernetesCache(),
	}
}

// kubeconfig gets a kubeconfig file.
func (k *Kubernetes) kubeconfig() (*rest.Config, error) {
	return rest.InClusterConfig()
}

// clientset creates a new Kubernetes client
func (k *Kubernetes) clientset() (*kubernetes.Clientset, error) {
	if k.client != nil {
		return k.client, nil
	}
	if k.config == nil {
		cfg, err := k.kubeconfig()
		if err != nil {
			return nil, err
		}
		k.config = cfg
	}

	// Log rate limiter settings before creating client
	if k.config.RateLimiter != nil {
		k.logger.Info("Kubernetes client has custom rate limiter configured")
	}

	// Log QPS and Burst settings
	if k.config.QPS > 0 || k.config.Burst > 0 {
		k.logger.Infof("Kubernetes client rate limiter settings - QPS: %.2f, Burst: %d",
			k.config.QPS, k.config.Burst)
	} else {
		k.logger.Info("Kubernetes client using default rate limiter settings")
	}

	cli, err := kubernetes.NewForConfig(k.config)
	if err != nil {
		k.logger.Errorf("Failed to create Kubernetes client: %v", err)
		return nil, err
	}
	k.client = cli
	return k.client, nil
}

// getJobsByIdentifier returns a list of jobs bound to the given testrun identifier.
func (k *Kubernetes) getJobsByIdentifier(ctx context.Context, client *kubernetes.Clientset, identifier string) (*v1.JobList, error) {
	// Get all jobs from cache or API
	allJobs, err := k.cache.getAllJobs(ctx, client, k.namespace, k.logger)
	if err != nil {
		return nil, err
	}

	// Filter jobs by identifier in-memory
	filteredJobs := &v1.JobList{
		TypeMeta: allJobs.TypeMeta,
		ListMeta: allJobs.ListMeta,
		Items:    []v1.Job{},
	}

	// Try different labels for backward compatibility:
	// - etos.eiffel-community.github.io/id is v1alpha+
	// - id is v0 legacy
	labelKeys := []string{"etos.eiffel-community.github.io/id", "id"}

	for _, job := range allJobs.Items {
		for _, labelKey := range labelKeys {
			if labelValue, exists := job.Labels[labelKey]; exists && labelValue == identifier {
				filteredJobs.Items = append(filteredJobs.Items, job)
				break // Found match, no need to check other labels for this job
			}
		}
	}

	k.logger.Infof("Filtered %d jobs with identifier '%s' from %d total jobs",
		len(filteredJobs.Items), identifier, len(allJobs.Items))

	return filteredJobs, nil
}

// IsFinished checks if an ESR job is finished.
func (k *Kubernetes) IsFinished(ctx context.Context, identifier string) bool {
	client, err := k.clientset()
	if err != nil {
		k.logger.Error(err)
		return false
	}
	jobs, err := k.getJobsByIdentifier(ctx, client, identifier)
	if err != nil {
		k.logger.Error(err)
		return false
	}
	if len(jobs.Items) == 0 {
		// Assume that a job is finished if it does not exist.
		k.logger.Warningf("job with id %s does not exist, assuming finished", identifier)
		return true
	}
	job := jobs.Items[0]
	if job.Status.Succeeded == 0 && job.Status.Failed == 0 {
		return false
	}
	return true
}

// LogListenerIP gets the IP address of an ESR log listener.
func (k *Kubernetes) LogListenerIP(ctx context.Context, identifier string) (string, error) {
	client, err := k.clientset()
	if err != nil {
		return "", err
	}
	jobs, err := k.getJobsByIdentifier(ctx, client, identifier)
	if err != nil {
		k.logger.Error(err)
		return "", err
	}
	if len(jobs.Items) == 0 {
		return "", fmt.Errorf("could not find esr job with id %s", identifier)
	}
	job := jobs.Items[0]

	// Get all pods from cache or API
	allPods, err := k.cache.getAllPods(ctx, client, k.namespace, k.logger)
	if err != nil {
		return "", err
	}

	// Filter pods by job name in-memory
	var matchingPods []corev1.Pod
	for _, pod := range allPods.Items {
		if jobName, exists := pod.Labels["job-name"]; exists && jobName == job.Name {
			matchingPods = append(matchingPods, pod)
		}
	}

	if len(matchingPods) == 0 {
		return "", fmt.Errorf("could not find pod for job with id %s", identifier)
	}

	k.logger.Infof("Found %d pods for job '%s' with identifier '%s'",
		len(matchingPods), job.Name, identifier)

	pod := matchingPods[0]
	return pod.Status.PodIP, nil
}
