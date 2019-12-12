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

package awssqs

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/whynowy/knative-source-awssqs/pkg/apis/sources/v1alpha1"
	"github.com/whynowy/knative-source-awssqs/pkg/reconciler/resources"

	"github.com/knative/eventing-sources/pkg/controller/sdk"
	"github.com/knative/eventing-sources/pkg/controller/sinks"
	"github.com/knative/pkg/logging"

	"go.uber.org/zap"
	"k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// controllerAgentName is the string used by this controller to
	// identify itself when creating events.
	controllerAgentName = "aws-sqs-source-controller"

	// raImageEnvVar is the name of the environment variable that
	// contains the receive adapter's image. It must be defined.
	raImageEnvVar = "AWSSQS_RA_IMAGE"

	finalizerName = controllerAgentName
)

// Add creates a new AwsSqsSource Controller and adds it to the Manager with
// default RBAC. The Manager will set fields on the Controller and Start it when
// the Manager is Started.
func Add(mgr manager.Manager, logger *zap.SugaredLogger) error {
	raImage, defined := os.LookupEnv(raImageEnvVar)
	if !defined {
		return fmt.Errorf("required environment variable '%s' not defined", raImageEnvVar)
	}

	log.Println("Adding the AWS SQS Source controller.")
	p := &sdk.Provider{
		AgentName: controllerAgentName,
		Parent:    &v1alpha1.AwsSqsSource{},
		Owns:      []runtime.Object{&v1.Deployment{}},
		Reconciler: &reconciler{
			scheme:              mgr.GetScheme(),
			receiveAdapterImage: raImage,
		},
	}

	return p.Add(mgr, logger)
}

type reconciler struct {
	client client.Client
	scheme *runtime.Scheme

	receiveAdapterImage string
}

func (r *reconciler) InjectClient(c client.Client) error {
	r.client = c
	return nil
}

func (r *reconciler) Reconcile(ctx context.Context, object runtime.Object) error {
	logger := logging.FromContext(ctx).Desugar()

	var err error

	src, ok := object.(*v1alpha1.AwsSqsSource)
	if !ok {
		logger.Error("could not find AwsSqs source", zap.Any("object", object))
		return nil
	}

	// This Source attempts to reconcile three things.
	// 1. Determine the sink's URI.
	//     - Nothing to delete.
	// 2. Create a receive adapter in the form of a Deployment.
	//     - Will be garbage collected by K8s when this AwsSqsSource is deleted.

	// See if the source has been deleted.
	deletionTimestamp := src.DeletionTimestamp
	if deletionTimestamp != nil {
		r.removeFinalizer(src)
		return nil
	}

	r.addFinalizer(src)

	src.Status.InitializeConditions()

	sinkURI, err := sinks.GetSinkURI(ctx, r.client, src.Spec.Sink, src.Namespace)
	if err != nil {
		src.Status.MarkNoSink("NotFound", "")
		return err
	}
	src.Status.MarkSink(sinkURI)

	_, err = r.createReceiveAdapter(ctx, src, sinkURI)
	if err != nil {
		logger.Error("Unable to create the receive adapter", zap.Error(err))
		return err
	}
	src.Status.MarkDeployed()

	return nil
}

func (r *reconciler) createReceiveAdapter(ctx context.Context, src *v1alpha1.AwsSqsSource, sinkURI string) (*v1.Deployment, error) {
	ra, err := r.getReceiveAdapter(ctx, src)
	if err != nil && !apierrors.IsNotFound(err) {
		logging.FromContext(ctx).Error("Unable to get an existing receive adapter", zap.Error(err))
		return nil, err
	}

	if len(src.Spec.AwsCredsSecret.Name) == 0 && len(src.Spec.AwsCredsSecret.Key) == 0 && len(src.Spec.KIAMOptions.AssignedIAMRole) == 0 && len(src.Spec.KIAMOptions.SqsIAMRoleARN) == 0 {
		logging.FromContext(ctx).Error("Neither AwsCredsSecret nor KIAMOptions has valid configuration.")
		return nil, fmt.Errorf("Configuration error")
	}

	queueName, region, err := parseQueueInfo(src.Spec.QueueURLOrARN)
	if err != nil {
		logging.FromContext(ctx).Error("Invaid SQS queue ARN or URL", zap.Error(err))
		return nil, err
	}

	adapterArgs := resources.ReceiveAdapterArgs{
		Image:     r.receiveAdapterImage,
		Source:    src,
		QueueName: queueName,
		Region:    region,
		Labels:    getLabels(src),
		SinkURI:   sinkURI,
	}

	expected := resources.MakeReceiveAdapter(&adapterArgs)
	if ra != nil {
		if r.podSpecChanged(ra.Spec.Template.Spec, expected.Spec.Template.Spec) {
			ra.Spec.Template.Spec = expected.Spec.Template.Spec
			if err = r.client.Update(ctx, ra); err != nil {
				return ra, err
			}
			logging.FromContext(ctx).Desugar().Info("Receive Adapter updated.", zap.Any("receiveAdapter", ra))
		} else {
			logging.FromContext(ctx).Desugar().Info("Reusing existing receive adapter", zap.Any("receiveAdapter", ra))
		}
		return ra, nil
	}

	if err := controllerutil.SetControllerReference(src, expected, r.scheme); err != nil {
		return nil, err
	}

	if err = r.client.Create(ctx, expected); err != nil {
		return nil, err
	}
	logging.FromContext(ctx).Desugar().Info("Receive Adapter created.", zap.Error(err), zap.Any("receiveAdapter", expected))
	return expected, err
}

func (r *reconciler) podSpecChanged(oldPodSpec corev1.PodSpec, newPodSpec corev1.PodSpec) bool {
	if !equality.Semantic.DeepDerivative(newPodSpec, oldPodSpec) {
		return true
	}
	if len(oldPodSpec.Containers) != len(newPodSpec.Containers) {
		return true
	}
	for i := range newPodSpec.Containers {
		if !equality.Semantic.DeepEqual(newPodSpec.Containers[i].Env, oldPodSpec.Containers[i].Env) {
			return true
		}
	}
	return false
}

func (r *reconciler) getReceiveAdapter(ctx context.Context, src *v1alpha1.AwsSqsSource) (*v1.Deployment, error) {
	dl := &v1.DeploymentList{}
	err := r.client.List(ctx, &client.ListOptions{
		Namespace:     src.Namespace,
		LabelSelector: r.getLabelSelector(src),
		// TODO this is only needed by the fake client. Real K8s does not need it. Remove it once
		// the fake is fixed.
		Raw: &metav1.ListOptions{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1.SchemeGroupVersion.String(),
				Kind:       "Deployment",
			},
		},
	}, dl)

	if err != nil {
		logging.FromContext(ctx).Desugar().Error("Unable to list deployments: %v", zap.Error(err))
		return nil, err
	}
	for _, dep := range dl.Items {
		if metav1.IsControlledBy(&dep, src) {
			return &dep, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{}, "")
}

func (r *reconciler) getLabelSelector(src *v1alpha1.AwsSqsSource) labels.Selector {
	return labels.SelectorFromSet(getLabels(src))
}

func (r *reconciler) addFinalizer(s *v1alpha1.AwsSqsSource) {
	finalizers := sets.NewString(s.Finalizers...)
	finalizers.Insert(finalizerName)
	s.Finalizers = finalizers.List()
}

func (r *reconciler) removeFinalizer(s *v1alpha1.AwsSqsSource) {
	finalizers := sets.NewString(s.Finalizers...)
	finalizers.Delete(finalizerName)
	s.Finalizers = finalizers.List()
}

func getLabels(src *v1alpha1.AwsSqsSource) map[string]string {
	return map[string]string{
		"knative-eventing-source":      controllerAgentName,
		"knative-eventing-source-name": src.Name,
	}
}

func generateSubName(src *v1alpha1.AwsSqsSource) string {
	return fmt.Sprintf("knative-eventing_%s_%s_%s", src.Namespace, src.Name, src.UID)
}

// parseQueueInfo parses SQS queue ARN or URL to queue name and region.
func parseQueueInfo(queue string) (string, string, error) {
	if strings.HasPrefix(queue, "https://") {
		return url2QueneInfo(queue)
	} else if strings.HasPrefix(queue, "arn:aws:sqs:") {
		return arn2QueneInfo(queue)
	} else {
		return "", "", fmt.Errorf("Incorrect SQS queue infomation, it should be either queue ARN or queue URL, %s", queue)
	}
}

func url2QueneInfo(queueUrl string) (string, string, error) {
	trimmedUrl := strings.TrimPrefix(queueUrl, "https://")
	// sqs.us-east-1.amazonaws.com/182582413695/s3msgq
	splits := strings.Split(trimmedUrl, "/")
	if len(splits) != 3 {
		return "", "", fmt.Errorf("SQS queue URL incorrect, %s", queueUrl)
	}
	// sqs.us-east-1.amazonaws.com
	s := strings.Split(splits[0], ".")
	if len(s) != 4 {
		return "", "", fmt.Errorf("SQS queue URL incorrect, %s", queueUrl)
	}
	return splits[2], s[1], nil
}

func arn2QueneInfo(queueArn string) (string, string, error) {
	// arn:aws:sqs:us-east-1:182582413695:s3msgq
	splits := strings.Split(queueArn, ":")
	if len(splits) != 6 {
		return "", "", fmt.Errorf("SQS queue ARN incorrect, %s", queueArn)
	}
	return splits[5], splits[3], nil
}
