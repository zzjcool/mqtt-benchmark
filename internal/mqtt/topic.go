package mqtt

import (
	"fmt"
	"strings"
)

// TopicGenerator handles topic template generation
type TopicGenerator struct {
	TopicTemplate string
	TopicNum      int
	ClientIndex   uint32
	CurrentIndex  int
}

// NewTopicGenerator creates a new TopicGenerator
func NewTopicGenerator(topicTemplate string, topicNum int, clientIndex uint32) *TopicGenerator {
	return &TopicGenerator{
		TopicTemplate: topicTemplate,
		TopicNum:     topicNum,
		ClientIndex:  clientIndex,
		CurrentIndex: 0,
	}
}

// ValidateTopicTemplate validates if the topic template is valid
func ValidateTopicTemplate(topicTemplate string, topicNum int) error {
	if topicNum > 1 && !strings.Contains(topicTemplate, "%d") {
		return fmt.Errorf("topic template must contain %%d when topic-num is greater than 1")
	}
	return nil
}

// NextTopic generates the next topic in the sequence
func (g *TopicGenerator) NextTopic() string {
	topic := g.TopicTemplate
	topic = strings.ReplaceAll(topic, "%i", fmt.Sprintf("%d", g.ClientIndex))
	
	if g.TopicNum > 0 {
		topic = strings.ReplaceAll(topic, "%d", fmt.Sprintf("%d", g.CurrentIndex))
		g.CurrentIndex = (g.CurrentIndex + 1) % g.TopicNum
	}
	
	return topic
}

// GetTopic returns the current topic without advancing the index
func (g *TopicGenerator) GetTopic() string {
	topic := g.TopicTemplate
	topic = strings.ReplaceAll(topic, "%i", fmt.Sprintf("%d", g.ClientIndex))
	
	if g.TopicNum > 0 {
		topic = strings.ReplaceAll(topic, "%d", fmt.Sprintf("%d", g.CurrentIndex))
	}
	
	return topic
}

// GetTopics returns all topics for this generator
func (g *TopicGenerator) GetTopics() []string {
	if g.TopicNum <= 0 {
		return []string{strings.ReplaceAll(g.TopicTemplate, "%i", fmt.Sprintf("%d", g.ClientIndex))}
	}

	topics := make([]string, g.TopicNum)
	for i := 0; i < g.TopicNum; i++ {
		topic := g.TopicTemplate
		topic = strings.ReplaceAll(topic, "%i", fmt.Sprintf("%d", g.ClientIndex))
		topic = strings.ReplaceAll(topic, "%d", fmt.Sprintf("%d", i))
		topics[i] = topic
	}
	return topics
}
