package memory

import (
	"sync"

	"github.com/openai/openai-go"
)

var manager *memoryManager

type memoryManager struct {
	sync.Mutex
	conversationId2Memory map[string]*ChatMemory
}

func FetchMemory(conversationId string) *ChatMemory {
	manager.Lock()
	defer manager.Unlock()

	if memory, ok := manager.conversationId2Memory[conversationId]; ok {
		return memory
	} else {
		memory = NewChatMemory()
		manager.conversationId2Memory[conversationId] = memory
		return memory
	}
}

func DeleteMemory(conversationId string) {
	manager.Lock()
	defer manager.Unlock()

	if memory, ok := manager.conversationId2Memory[conversationId]; ok {
		memory.messages = make([]openai.ChatCompletionMessageParamUnion, 0, 0)
		memory.toolCallId2Name = make(map[string]string)
		delete(manager.conversationId2Memory, conversationId)
	}
}

func init() {
	manager = &memoryManager{
		conversationId2Memory: make(map[string]*ChatMemory),
	}
}
