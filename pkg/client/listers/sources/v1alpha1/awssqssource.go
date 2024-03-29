/*
Copyright 2019

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/whynowy/knative-source-awssqs/pkg/apis/sources/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// AwsSqsSourceLister helps list AwsSqsSources.
type AwsSqsSourceLister interface {
	// List lists all AwsSqsSources in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.AwsSqsSource, err error)
	// AwsSqsSources returns an object that can list and get AwsSqsSources.
	AwsSqsSources(namespace string) AwsSqsSourceNamespaceLister
	AwsSqsSourceListerExpansion
}

// awsSqsSourceLister implements the AwsSqsSourceLister interface.
type awsSqsSourceLister struct {
	indexer cache.Indexer
}

// NewAwsSqsSourceLister returns a new AwsSqsSourceLister.
func NewAwsSqsSourceLister(indexer cache.Indexer) AwsSqsSourceLister {
	return &awsSqsSourceLister{indexer: indexer}
}

// List lists all AwsSqsSources in the indexer.
func (s *awsSqsSourceLister) List(selector labels.Selector) (ret []*v1alpha1.AwsSqsSource, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.AwsSqsSource))
	})
	return ret, err
}

// AwsSqsSources returns an object that can list and get AwsSqsSources.
func (s *awsSqsSourceLister) AwsSqsSources(namespace string) AwsSqsSourceNamespaceLister {
	return awsSqsSourceNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// AwsSqsSourceNamespaceLister helps list and get AwsSqsSources.
type AwsSqsSourceNamespaceLister interface {
	// List lists all AwsSqsSources in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.AwsSqsSource, err error)
	// Get retrieves the AwsSqsSource from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.AwsSqsSource, error)
	AwsSqsSourceNamespaceListerExpansion
}

// awsSqsSourceNamespaceLister implements the AwsSqsSourceNamespaceLister
// interface.
type awsSqsSourceNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all AwsSqsSources in the indexer for a given namespace.
func (s awsSqsSourceNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.AwsSqsSource, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.AwsSqsSource))
	})
	return ret, err
}

// Get retrieves the AwsSqsSource from the indexer for a given namespace and name.
func (s awsSqsSourceNamespaceLister) Get(name string) (*v1alpha1.AwsSqsSource, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("awssqssource"), name)
	}
	return obj.(*v1alpha1.AwsSqsSource), nil
}
