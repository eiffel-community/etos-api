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
	"sync"
	"time"

	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Cache entry with TTL
type cacheEntry struct {
	data      interface{}
	timestamp time.Time
}

// Cache for Kubernetes API responses with TTL
type kubernetesCache struct {
	jobs     sync.Map      // map[string]*cacheEntry for job lists
	pods     sync.Map      // map[string]*cacheEntry for pod lists
	cacheTTL time.Duration // Cache validity duration
	// Mutexes to prevent concurrent API calls for the same resource
	jobsMutex sync.Mutex
	podsMutex sync.Mutex
}

// newKubernetesCache creates a new cache with configured cache validity
func newKubernetesCache() *kubernetesCache {
	return &kubernetesCache{
		cacheTTL: 5 * time.Second,
	}
}

// getAllJobs retrieves all jobs from cache or API, making API calls if cached data is stale
func (c *kubernetesCache) getAllJobs(ctx context.Context, client *kubernetes.Clientset, namespace string, logger *logrus.Entry) (*v1.JobList, error) {
	// Use namespace as cache key since we're caching all jobs in the namespace
	key := fmt.Sprintf("all_jobs_%s", namespace)

	// Nested function to check cache and return data if fresh
	checkCache := func() (*v1.JobList, bool) {
		if cached, ok := c.jobs.Load(key); ok {
			if entry, ok := cached.(*cacheEntry); ok {
				if time.Since(entry.timestamp) < c.cacheTTL {
					if jobs, ok := entry.data.(*v1.JobList); ok {
						return jobs, true
					}
				}
			}
		}
		return nil, false
	}

	// Check cache first (fast path - no locking)
	if jobs, found := checkCache(); found {
		logger.Infof("Returning cached jobs for namespace: %s (age: %v, count: %d)", namespace, time.Since(getTimestamp(&c.jobs, key)), len(jobs.Items))
		return jobs, nil
	}

	// Use mutex to prevent concurrent API calls
	c.jobsMutex.Lock()
	defer c.jobsMutex.Unlock()

	// Double-check cache after acquiring mutex (another goroutine might have updated it)
	if jobs, found := checkCache(); found {
		logger.Infof("Returning cached jobs for namespace: %s (age: %v, count: %d) [double-check]", namespace, time.Since(getTimestamp(&c.jobs, key)), len(jobs.Items))
		return jobs, nil
	}

	// Fetch from API if no cache entry exists or cached data is stale
	logger.Infof("Making Kubernetes API call to fetch all jobs for namespace: %s", namespace)
	jobs, err := client.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Errorf("Failed to fetch jobs from Kubernetes API for namespace %s: %v", namespace, err)
		return nil, err
	}

	// Store in cache
	c.jobs.Store(key, &cacheEntry{
		data:      jobs,
		timestamp: time.Now(),
	})

	logger.Infof("Successfully fetched and cached %d jobs for namespace: %s", len(jobs.Items), namespace)
	return jobs, nil
}

// getAllPods retrieves all pods from cache or API, making API calls if cached data is stale
func (c *kubernetesCache) getAllPods(ctx context.Context, client *kubernetes.Clientset, namespace string, logger *logrus.Entry) (*corev1.PodList, error) {
	// Use namespace as cache key since we're caching all pods in the namespace
	key := fmt.Sprintf("all_pods_%s", namespace)

	// Nested function to check cache and return data if fresh
	checkCache := func() (*corev1.PodList, bool) {
		if cached, ok := c.pods.Load(key); ok {
			if entry, ok := cached.(*cacheEntry); ok {
				if time.Since(entry.timestamp) < c.cacheTTL {
					if pods, ok := entry.data.(*corev1.PodList); ok {
						return pods, true
					}
				}
			}
		}
		return nil, false
	}

	// Check cache first (fast path - no locking)
	if pods, found := checkCache(); found {
		logger.Infof("Returning cached pods for namespace: %s (age: %v, count: %d)", namespace, time.Since(getTimestamp(&c.pods, key)), len(pods.Items))
		return pods, nil
	}

	// Use mutex to prevent concurrent API calls
	c.podsMutex.Lock()
	defer c.podsMutex.Unlock()

	// Double-check cache after acquiring mutex (another goroutine might have updated it)
	if pods, found := checkCache(); found {
		logger.Infof("Returning cached pods for namespace: %s (age: %v, count: %d) [double-check]", namespace, time.Since(getTimestamp(&c.pods, key)), len(pods.Items))
		return pods, nil
	}

	// Fetch from API if no cache entry exists or cached data is stale
	logger.Infof("Making Kubernetes API call to fetch all pods for namespace: %s", namespace)
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Errorf("Failed to fetch pods from Kubernetes API for namespace %s: %v", namespace, err)
		return nil, err
	}

	// Store in cache
	c.pods.Store(key, &cacheEntry{
		data:      pods,
		timestamp: time.Now(),
	})

	logger.Infof("Successfully fetched and cached %d pods for namespace: %s", len(pods.Items), namespace)
	return pods, nil
}

// getTimestamp is a helper function to get the timestamp of a cache entry
func getTimestamp(cache *sync.Map, key string) time.Time {
	if cached, ok := cache.Load(key); ok {
		if entry, ok := cached.(*cacheEntry); ok {
			return entry.timestamp
		}
	}
	return time.Time{}
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
