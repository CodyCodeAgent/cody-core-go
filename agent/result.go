package agent

import (
	"io"
	"sync"

	"github.com/cloudwego/eino/schema"
)

// Result contains the output of an agent run.
type Result[O any] struct {
	// Output is the typed structured output.
	Output O
	// Usage contains token usage statistics for this run.
	Usage Usage
	// allMessages contains all messages including history (excluding system messages).
	allMessages []*schema.Message
	// newMessageStart is the index where new messages begin in allMessages.
	newMessageStart int
}

// NewMessages returns only the messages generated during this run
// (excludes message_history and system messages).
// Returns an empty slice (not nil) if there are no new messages.
func (r *Result[O]) NewMessages() []*schema.Message {
	if r.newMessageStart >= len(r.allMessages) {
		return []*schema.Message{}
	}
	return r.allMessages[r.newMessageStart:]
}

// AllMessages returns the complete message sequence including history
// (excludes system messages).
func (r *Result[O]) AllMessages() []*schema.Message {
	return r.allMessages
}

// conversationRef holds a reference back to a conversation for auto-updating
// history after streaming completes.
type conversationRef struct {
	setMessages func([]*schema.Message)
}

// StreamResult provides streaming access to an agent's output.
// Text chunks are forwarded in real-time via TextStream() as they arrive from the model.
// Call Final() to wait for the agent loop to complete and get the typed result.
type StreamResult[O any] struct {
	stream      *schema.StreamReader[*schema.Message]
	textCh      chan string
	finalResult *Result[O]
	finalErr    error
	done        chan struct{}
	once        sync.Once

	// conv is set when created via Conversation.SendStream to auto-update history.
	conv *conversationRef

	// For internal use by the agent loop
	agentLoop func()
}

// TextStream returns a channel that yields text chunks as they arrive.
func (s *StreamResult[O]) TextStream() <-chan string {
	s.once.Do(func() {
		s.textCh = make(chan string, 64)
		s.done = make(chan struct{})
		go func() {
			defer close(s.textCh)
			defer close(s.done)
			if s.agentLoop != nil {
				s.agentLoop()
				return
			}
			// Simple stream forwarding
			for {
				msg, err := s.stream.Recv()
				if err != nil {
					if err != io.EOF {
						s.finalErr = err
					}
					return
				}
				if msg.Content != "" {
					s.textCh <- msg.Content
				}
			}
		}()
	})
	return s.textCh
}

// Final waits for the stream to complete and returns the final result.
// If the StreamResult was created via Conversation.SendStream, Final automatically
// updates the conversation history upon success.
func (s *StreamResult[O]) Final() (*Result[O], error) {
	// Ensure streaming has started
	s.TextStream()
	// Drain the text channel
	for range s.textCh {
	}
	<-s.done
	if s.finalErr != nil {
		return nil, s.finalErr
	}
	// Auto-update conversation history if linked to a conversation
	if s.conv != nil && s.finalResult != nil {
		s.conv.setMessages(s.finalResult.AllMessages())
	}
	return s.finalResult, nil
}

// Close releases resources associated with the stream.
func (s *StreamResult[O]) Close() {
	if s.stream != nil {
		s.stream.Close()
	}
}
