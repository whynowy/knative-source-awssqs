# Knative AWS SQS Event Source

AWS SQS event source is a [Knative](https://github.com/knative/) addon, it can
be used to listen to an AWS SQS queue, and deliver events to an
[Addressable](https://github.com/knative/eventing/tree/master/docs/spec/interfaces.md#addressable)
target.

## What's Different

Different from other Knative SQS event source implementation, this one provides
more features.

1. Multiple SQS auth approaches.
   - Credential in Kubernetes secret
   - AWS IAM AssumeRole supported by [KIAM](https://github.com/uswitch/kiam)
1. Message single delivery or bulk delivery to downstream addressable.
1. Multiple workers

Follow the [samples](samples) to try out this SQS event source.
