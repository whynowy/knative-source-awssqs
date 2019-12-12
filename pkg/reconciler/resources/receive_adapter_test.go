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

package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/whynowy/knative-source-awssqs/pkg/apis/sources/v1alpha1"
	"k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	credsVolume    = "aws-credentials"
	credsMountPath = "/var/secrets/aws"
)

func TestMakeReceiveAdapterCredential(t *testing.T) {
	src := &v1alpha1.AwsSqsSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "source-name",
			Namespace: "source-namespace",
		},
		Spec: v1alpha1.AwsSqsSourceSpec{
			ServiceAccountName: "source-svc-acct",
			QueueURLOrARN:      "aws-sqs-queue",
			AwsCredsSecret: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "sqs-secret-name",
				},
				Key: "sqs-secret-key",
			},
			Workers:      5,
			BulkDelivery: false,
		},
	}

	got := MakeReceiveAdapter(&ReceiveAdapterArgs{
		Image:     "test-image",
		Source:    src,
		QueueName: "queue-name",
		Region:    "region",
		Labels: map[string]string{
			"test-key1": "test-value1",
			"test-key2": "test-value2",
		},
		SinkURI: "sink-uri",
	})

	one := int32(1)
	want := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    "source-namespace",
			GenerateName: "awssqs-source-name-",
			Labels: map[string]string{
				"test-key1": "test-value1",
				"test-key2": "test-value2",
			},
		},
		Spec: v1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"test-key1": "test-value1",
					"test-key2": "test-value2",
				},
			},
			Replicas: &one,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"sidecar.istio.io/inject": "true",
					},
					Labels: map[string]string{
						"test-key1": "test-value1",
						"test-key2": "test-value2",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "source-svc-acct",
					Containers: []corev1.Container{
						{
							Name:  "receive-adapter",
							Image: "test-image",
							Env: []corev1.EnvVar{
								{
									Name:  "AWS_APPLICATION_CREDENTIALS",
									Value: "/var/secrets/aws/sqs-secret-key",
								},
								{
									Name:  "SQS_QUEUE_NAME",
									Value: "queue-name",
								},
								{
									Name:  "SQS_REGION",
									Value: "region",
								},
								{
									Name:  "WORKERS",
									Value: "5",
								},
								{
									Name:  "BULK_DELIVERY",
									Value: "false",
								},
								{
									Name:  "SINK_URI",
									Value: "sink-uri",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      credsVolume,
									MountPath: credsMountPath,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: credsVolume,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "sqs-secret-name",
								},
							},
						},
					},
				},
			},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected deploy (-want, +got) = %v", diff)
	}
}

func TestMakeReceiveAdapterKiam(t *testing.T) {
	src := &v1alpha1.AwsSqsSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "source-name",
			Namespace: "source-namespace",
		},
		Spec: v1alpha1.AwsSqsSourceSpec{
			ServiceAccountName: "source-svc-acct",
			QueueURLOrARN:      "aws-sqs-queue",
			KIAMOptions: v1alpha1.KiamOptions{
				AssignedIAMRole: "assigned-role",
				SqsIAMRoleARN:   "sqs-role",
			},
			Workers:      5,
			BulkDelivery: false,
		},
	}

	got := MakeReceiveAdapter(&ReceiveAdapterArgs{
		Image:     "test-image",
		Source:    src,
		QueueName: "queue-name",
		Region:    "region",
		Labels: map[string]string{
			"test-key1": "test-value1",
			"test-key2": "test-value2",
		},
		SinkURI: "sink-uri",
	})

	one := int32(1)
	want := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    "source-namespace",
			GenerateName: "awssqs-source-name-",
			Labels: map[string]string{
				"test-key1": "test-value1",
				"test-key2": "test-value2",
			},
		},
		Spec: v1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"test-key1": "test-value1",
					"test-key2": "test-value2",
				},
			},
			Replicas: &one,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"sidecar.istio.io/inject": "true",
						"iam.amazonaws.com/role":  "assigned-role",
					},
					Labels: map[string]string{
						"test-key1": "test-value1",
						"test-key2": "test-value2",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "source-svc-acct",
					Containers: []corev1.Container{
						{
							Name:  "receive-adapter",
							Image: "test-image",
							Env: []corev1.EnvVar{
								{
									Name:  "SQS_QUEUE_NAME",
									Value: "queue-name",
								},
								{
									Name:  "SQS_IAM_ROLE_ARN",
									Value: "sqs-role",
								},
								{
									Name:  "SQS_REGION",
									Value: "region",
								},
								{
									Name:  "WORKERS",
									Value: "5",
								},
								{
									Name:  "BULK_DELIVERY",
									Value: "false",
								},
								{
									Name:  "SINK_URI",
									Value: "sink-uri",
								},
							},
						},
					},
				},
			},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected deploy (-want, +got) = %v", diff)
	}
}
