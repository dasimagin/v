package main

// screen is a saved main area: what Esc returns to.
type screen struct {
	mode      mode
	doc       *Doc
	browser   *Browser
	top, left int
}

// push saves the current screen before opening another one on top.
func (m *model) push() {
	m.stack = append(m.stack, screen{m.mode, m.doc, m.browser, m.top, m.left})
}

// pop returns to the previous screen, or to the chat when there is none.
func (m *model) pop() {
	if len(m.stack) == 0 {
		m.mode = modeChat
		return
	}
	s := m.stack[len(m.stack)-1]
	m.stack = m.stack[:len(m.stack)-1]
	m.mode, m.doc, m.browser, m.top, m.left = s.mode, s.doc, s.browser, s.top, s.left
}
