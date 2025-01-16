//go:build h2

package fns

import (
	"errors"
	"sync"
)

var ErrParentNotFound = errors.New("parent stream not found")
var ErrStreamNotFound = errors.New("stream not found")

// streamNode represents a node in the priority tree
type streamNode struct {
	stream    *h2Stream
	children  []*streamNode
	parent    *streamNode
	exclusive bool
}

// StreamScheduler manages the priority of HTTP/2 streams
type StreamScheduler struct {
	root  *streamNode
	nodes []*streamNode // Array to store all nodes for quick access
	mu    sync.Mutex
}

// New creates a new StreamScheduler
func New() *StreamScheduler {
	return &StreamScheduler{
		root: &streamNode{}, // Root node with no stream
	}
}

// AddStream adds a new stream to the priority tree
func (p *StreamScheduler) AddStream(stream *h2Stream, parentID uint32, priority int32, exclusive bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Find parent node
	var parent *streamNode
	if parentID == 0 {
		parent = p.root
	} else {
		parent = p.findNodeByStreamID(parentID)
	}

	if parent == nil {
		return ErrParentNotFound
	}

	// Create new node
	newNode := &streamNode{stream: stream, exclusive: exclusive}

	// Handle exclusive flag
	if exclusive {
		for _, child := range parent.children {
			child.parent = newNode
			newNode.children = append(newNode.children, child)
		}
		parent.children = []*streamNode{newNode}
	} else {
		parent.children = append(parent.children, newNode)
	}

	newNode.parent = parent
	p.nodes = append(p.nodes, newNode)

	// Update priority
	stream.priority.Store(priority)

	return nil
}

// UpdateParent updates the parent of a stream
func (p *StreamScheduler) UpdateParent(stream *h2Stream, newParentID uint32, exclusive bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Get stream's node
	node := p.findNodeByStreamID(stream.id)
	if node == nil {
		return ErrStreamNotFound
	}

	// Ger the new parent's node
	newParent := p.findNodeByStreamID(newParentID)
	if newParent == nil {
		return ErrParentNotFound
	}

	// Remove node from its current parent
	if node.parent != nil {
		for i, child := range node.parent.children {
			if child == node {
				node.parent.children = append(node.parent.children[:i], node.parent.children[i+1:]...)
				break
			}
		}
	}

	// Set new parent in node
	node.parent = newParent

	// Set children in parent's node. If exclusive, move all siblings to the children collection
	if exclusive {
		// Move newParent's children to node children
		node.children = append(node.children, newParent.children...)
		for _, child := range newParent.children {
			child.parent = node
		}
		// Safe delete all newParent's children and append current node
		for i := range newParent.children {
			newParent.children[i] = nil
		}
		newParent.children = newParent.children[:0]
	}

	newParent.children = append(newParent.children, node)

	return nil
}

// GetReadyStream returns the most suitable stream for processing based on priority and tree structure
func (p *StreamScheduler) GetReadyStream(state StreamState) *h2Stream {
	var bestStream *h2Stream
	var bestPriority int32 = -1
	var bestDistance int = -1

	// Helper function to recursively traverse the tree
	var traverse func(*streamNode, int)
	traverse = func(node *streamNode, distance int) {
		if node == nil {
			return
		}

		// Check if the stream is in the correct state
		if node.stream.state == state {
			priority := node.stream.priority.Load()

			// Case 1: Stream without a parent
			if node.parent == nil && priority > bestPriority {
				bestStream = node.stream
				bestPriority = priority
				bestDistance = distance
			}

			// Case 2: Stream with a parent, considering distance
			if (bestStream == nil || priority > bestPriority || (priority == bestPriority && distance < bestDistance)) && node.parent != nil {
				bestStream = node.stream
				bestPriority = priority
				bestDistance = distance
			}
		}

		// Traverse children
		for _, child := range node.children {
			traverse(child, distance+1)
		}
	}

	// Start traversal from the root (node 0)
	traverse(p.root, 0)

	return bestStream
}

// Destroy releases objects and memory safely
func (p *StreamScheduler) Destroy() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Clear all references
	p.root = nil
	for i := range p.nodes {
		p.nodes[i].stream = nil
		p.nodes[i].children = nil
		p.nodes[i].parent = nil
		p.nodes[i] = nil
	}
	p.nodes = nil
}

// findNodeByID finds a node by its stream id. Ensure a lock is set before calling this function
// TODO: This is a naive implementation, we can improve the performance using binary search or other data structure
func (p *StreamScheduler) findNodeByStreamID(id uint32) *streamNode {
	// Return parent node when id=0
	if id == 0 {
		return p.root
	}

	// Return other nodes
	for _, node := range p.nodes {
		if node.stream != nil && node.stream.id == id {
			return node
		}
	}

	return nil
}
