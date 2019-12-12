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
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"go.uber.org/zap"
)

func TestPostSingleMessage_ServeHTTP(t *testing.T) {
	testCases := map[string]struct {
		sink              func(http.ResponseWriter, *http.Request)
		reqBody           string
		attributes        map[string]*string
		expectedEventType string
		error             bool
	}{
		"happy": {
			sink:       sinkAccepted,
			reqBody:    `{"Attributes":{"SentTimestamp":"1238099229000"},"Body":"{\"a\":\"b\"}","MD5OfBody":"MD5Body","MD5OfMessageAttributes":"MD5ATTRS","MessageAttributes":null,"MessageId":"ABC","ReceiptHandle":null}`,
			attributes: map[string]*string{"SentTimestamp": aws.String("1238099229000")},
		},
		"rejected": {
			sink:       sinkRejected,
			reqBody:    `{"Attributes":{"SentTimestamp":"1238099229000"},"Body":"{\"a\":\"b\"}","MD5OfBody":"MD5Body","MD5OfMessageAttributes":"MD5ATTRS","MessageAttributes":null,"MessageId":"ABC","ReceiptHandle":null}`,
			attributes: map[string]*string{"SentTimestamp": aws.String("1238099229000")},
			error:      true,
		},
	}
	for n, tc := range testCases {
		t.Run(n, func(t *testing.T) {
			h := &fakeHandler{
				handler: tc.sink,
			}
			sinkServer := httptest.NewServer(h)
			defer sinkServer.Close()

			a := &Adapter{
				QueueName:     "queue-name",
				Region:        "region",
				SinkURI:       sinkServer.URL,
				SqsIamRoleArn: "iam-role",

				queueURL: "quque-url",
			}

			if err := a.initClient(); err != nil {
				t.Errorf("failed to create cloudevent client, %v", err)
			}

			data, err := json.Marshal(map[string]string{"a": "b"})
			if err != nil {
				t.Errorf("unexpected error, %v", err)
			}

			m := &sqs.Message{
				MessageId:              aws.String("ABC"),
				Body:                   aws.String(string(data)),
				Attributes:             tc.attributes,
				MD5OfBody:              aws.String("MD5Body"),
				MD5OfMessageAttributes: aws.String("MD5ATTRS"),
			}
			err = a.postMessage(zap.S(), m)

			if tc.error && err == nil {
				t.Errorf("expected error, but got %v", err)
			}

			et := h.header.Get("Ce-Type") // bad bad bad.

			expectedEventType := eventType
			if tc.expectedEventType != "" {
				expectedEventType = tc.expectedEventType
			}

			if et != expectedEventType {
				t.Errorf("Expected eventtype %q, but got %q", tc.expectedEventType, et)
			}
			if tc.reqBody != string(h.body) {
				t.Errorf("expected request body %q, but got %q", tc.reqBody, h.body)
			}
		})
	}
}

func TestPostBulkMessages_ServeHTTP(t *testing.T) {
	testCases := map[string]struct {
		sink              func(http.ResponseWriter, *http.Request)
		reqBody           string
		attributes        map[string]*string
		expectedEventType string
		error             bool
	}{
		"happy": {
			sink:       sinkAccepted,
			reqBody:    `[{"Attributes":{"SentTimestamp":"1238099229000"},"Body":"{\"a\":\"b\"}","MD5OfBody":"MD5Body","MD5OfMessageAttributes":"MD5ATTRS","MessageAttributes":null,"MessageId":"ABC","ReceiptHandle":null}]`,
			attributes: map[string]*string{"SentTimestamp": aws.String("1238099229000")},
		},
		"rejected": {
			sink:       sinkRejected,
			reqBody:    `[{"Attributes":{"SentTimestamp":"1238099229000"},"Body":"{\"a\":\"b\"}","MD5OfBody":"MD5Body","MD5OfMessageAttributes":"MD5ATTRS","MessageAttributes":null,"MessageId":"ABC","ReceiptHandle":null}]`,
			attributes: map[string]*string{"SentTimestamp": aws.String("1238099229000")},
			error:      true,
		},
	}
	for n, tc := range testCases {
		t.Run(n, func(t *testing.T) {
			h := &fakeHandler{
				handler: tc.sink,
			}
			sinkServer := httptest.NewServer(h)
			defer sinkServer.Close()

			a := &Adapter{
				QueueName:     "queue-name",
				Region:        "region",
				SinkURI:       sinkServer.URL,
				SqsIamRoleArn: "iam-role",

				queueURL: "quque-url",
			}

			if err := a.initClient(); err != nil {
				t.Errorf("failed to create cloudevent client, %v", err)
			}

			data, err := json.Marshal(map[string]string{"a": "b"})
			if err != nil {
				t.Errorf("unexpected error, %v", err)
			}

			m := &sqs.Message{
				MessageId:              aws.String("ABC"),
				Body:                   aws.String(string(data)),
				Attributes:             tc.attributes,
				MD5OfBody:              aws.String("MD5Body"),
				MD5OfMessageAttributes: aws.String("MD5ATTRS"),
			}
			messages := []*sqs.Message{m}
			err = a.postMessages(zap.S(), messages)

			if tc.error && err == nil {
				t.Errorf("expected error, but got %v", err)
			}

			et := h.header.Get("Ce-Type") //

			expectedEventType := eventType
			if tc.expectedEventType != "" {
				expectedEventType = tc.expectedEventType
			}

			if et != expectedEventType {
				t.Errorf("Expected eventtype %q, but got %q", tc.expectedEventType, et)
			}
			if tc.reqBody != string(h.body) {
				t.Errorf("expected request body %q, but got %q", tc.reqBody, h.body)
			}
		})
	}
}

type fakeHandler struct {
	body   []byte
	header http.Header

	handler func(http.ResponseWriter, *http.Request)
}

func (h *fakeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.header = r.Header
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "can not read body", http.StatusBadRequest)
		return
	}
	h.body = body

	defer r.Body.Close()
	h.handler(w, r)
}

func sinkAccepted(writer http.ResponseWriter, req *http.Request) {
	writer.WriteHeader(http.StatusOK)
}

func sinkRejected(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusRequestTimeout)
}
