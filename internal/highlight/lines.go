package highlight

import "github.com/golang-gui/goui-sourceview/language"

// Line is document metadata, not another copy of the text.
type Line struct {
	ID                        uint64
	Bytes                     int // includes LF when LF is true
	LF                        bool
	in, out                   language.State
	complete, loaded, limited bool
	tokens                    []Token
	version                   uint64
}

type lineNode struct {
	left, right  *lineNode
	block        []*Line
	priority     uint64
	count, bytes int
}
type lineStore struct {
	root *lineNode
	seed uint64
}

func count(n *lineNode) int {
	if n == nil {
		return 0
	}
	return n.count
}
func byteCount(n *lineNode) int {
	if n == nil {
		return 0
	}
	return n.bytes
}
func (n *lineNode) update() {
	n.count = count(n.left) + len(n.block) + count(n.right)
	n.bytes = byteCount(n.left) + byteCount(n.right)
	for _, l := range n.block {
		n.bytes += l.Bytes
	}
}
func (s *lineStore) node(block []*Line) *lineNode {
	if len(block) == 0 {
		return nil
	}
	s.seed += 0x9e3779b97f4a7c15
	z := s.seed
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	n := &lineNode{block: block[:len(block):len(block)], priority: z ^ (z >> 31)}
	n.update()
	return n
}
func merge(a, b *lineNode) *lineNode {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if a.priority > b.priority {
		a.right = merge(a.right, b)
		a.update()
		return a
	}
	b.left = merge(a, b.left)
	b.update()
	return b
}
func (s *lineStore) split(n *lineNode, at int) (*lineNode, *lineNode) {
	if n == nil {
		return nil, nil
	}
	left := count(n.left)
	if at < left {
		a, b := s.split(n.left, at)
		n.left = b
		n.update()
		return a, n
	}
	if at > left+len(n.block) {
		a, b := s.split(n.right, at-left-len(n.block))
		n.right = a
		n.update()
		return n, b
	}
	cut := at - left
	return merge(n.left, s.node(n.block[:cut])), merge(s.node(n.block[cut:]), n.right)
}
func (s *lineStore) tree(lines []*Line) *lineNode {
	var result *lineNode
	for len(lines) > 0 {
		n := min(128, len(lines))
		block := append([]*Line(nil), lines[:n]...)
		result = merge(result, s.node(block))
		lines = lines[n:]
	}
	return result
}
func (s *lineStore) splice(at, removed int, lines []*Line) {
	a, rest := s.split(s.root, at)
	_, b := s.split(rest, removed)
	s.root = merge(merge(a, s.tree(lines)), b)
}
func (s *lineStore) at(index int) *Line {
	n := s.root
	for n != nil {
		left := count(n.left)
		if index < left {
			n = n.left
		} else if index < left+len(n.block) {
			return n.block[index-left]
		} else {
			index -= left + len(n.block)
			n = n.right
		}
	}
	return nil
}
func (s *lineStore) offset(index int) int {
	n := s.root
	result := 0
	for n != nil {
		left := count(n.left)
		if index < left {
			n = n.left
			continue
		}
		result += byteCount(n.left)
		index -= left
		if index <= len(n.block) {
			for _, l := range n.block[:index] {
				result += l.Bytes
			}
			return result
		}
		result += n.bytes - byteCount(n.left) - byteCount(n.right)
		index -= len(n.block)
		n = n.right
	}
	return result
}
func (s *lineStore) lineAt(pos int) int {
	if pos >= byteCount(s.root) {
		return max(0, count(s.root)-1)
	}
	n := s.root
	index := 0
	for n != nil {
		if pos < byteCount(n.left) {
			n = n.left
			continue
		}
		pos -= byteCount(n.left)
		index += count(n.left)
		blockBytes := n.bytes - byteCount(n.left) - byteCount(n.right)
		if pos >= blockBytes {
			pos -= blockBytes
			index += len(n.block)
			n = n.right
			continue
		}
		for _, l := range n.block {
			if pos < l.Bytes {
				return index
			}
			pos -= l.Bytes
			index++
		}
		n = n.right
	}
	return max(0, count(s.root)-1)
}
