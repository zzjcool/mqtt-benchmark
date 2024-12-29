package mqtt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTopicGenerator(t *testing.T) {
	tests := []struct {
		name          string
		topicTemplate string
		topicNum      int
		clientIndex   uint32
		wantTopics    []string
		wantError     bool
	}{
		{
			name:          "Single topic without placeholders",
			topicTemplate: "test/topic",
			topicNum:      0,
			clientIndex:   0,
			wantTopics:    []string{"test/topic"},
			wantError:     false,
		},
		{
			name:          "Single topic with client index",
			topicTemplate: "test/topic/%i",
			topicNum:      0,
			clientIndex:   1,
			wantTopics:    []string{"test/topic/1"},
			wantError:     false,
		},
		{
			name:          "Multiple topics with sequence",
			topicTemplate: "test/topic/%d",
			topicNum:      3,
			clientIndex:   0,
			wantTopics:    []string{"test/topic/0", "test/topic/1", "test/topic/2"},
			wantError:     false,
		},
		{
			name:          "Multiple topics with client index and sequence",
			topicTemplate: "test/client/%i/topic/%d",
			topicNum:      2,
			clientIndex:   5,
			wantTopics:    []string{"test/client/5/topic/0", "test/client/5/topic/1"},
			wantError:     false,
		},
		{
			name:          "Invalid template for multiple topics",
			topicTemplate: "test/topic",
			topicNum:      2,
			clientIndex:   0,
			wantTopics:    nil,
			wantError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test topic template validation
			err := ValidateTopicTemplate(tt.topicTemplate, tt.topicNum)
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Create topic generator
			generator := NewTopicGenerator(tt.topicTemplate, tt.topicNum, tt.clientIndex)
			assert.NotNil(t, generator)

			// Test GetTopics
			topics := generator.GetTopics()
			assert.Equal(t, tt.wantTopics, topics)

			// Test NextTopic and GetTopic
			if tt.topicNum > 0 {
				for i := 0; i < tt.topicNum*2; i++ { // Test cycling through topics twice
					nextTopic := generator.NextTopic()
					expectedTopic := tt.wantTopics[i%tt.topicNum]
					assert.Equal(t, expectedTopic, nextTopic)

					// GetTopic should return the same topic that was just generated
					currentTopic := generator.GetTopic()
					assert.Equal(t, expectedTopic, currentTopic)
				}
			} else {
				// For single topics, NextTopic and GetTopic should always return the same value
				nextTopic := generator.NextTopic()
				assert.Equal(t, tt.wantTopics[0], nextTopic)
				currentTopic := generator.GetTopic()
				assert.Equal(t, tt.wantTopics[0], currentTopic)
			}
		})
	}
}

func TestTopicGeneratorEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		topicTemplate string
		topicNum      int
		clientIndex   uint32
		testFunc      func(*testing.T, *TopicGenerator)
	}{
		{
			name:          "Empty template",
			topicTemplate: "",
			topicNum:      0,
			clientIndex:   0,
			testFunc: func(t *testing.T, g *TopicGenerator) {
				assert.Equal(t, "", g.NextTopic())
				assert.Equal(t, "", g.GetTopic())
				assert.Equal(t, []string{""}, g.GetTopics())
			},
		},
		{
			name:          "Negative topic number",
			topicTemplate: "test/%d",
			topicNum:      -1,
			clientIndex:   0,
			testFunc: func(t *testing.T, g *TopicGenerator) {
				assert.Equal(t, []string{"test/%d"}, g.GetTopics())
			},
		},
		{
			name:          "Multiple consecutive placeholders",
			topicTemplate: "test/%i/%i/%d/%d",
			topicNum:      2,
			clientIndex:   1,
			testFunc: func(t *testing.T, g *TopicGenerator) {
				expected := []string{
					"test/1/1/0/0",
					"test/1/1/1/1",
				}
				assert.Equal(t, expected, g.GetTopics())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := NewTopicGenerator(tt.topicTemplate, tt.topicNum, tt.clientIndex)
			tt.testFunc(t, generator)
		})
	}
}
