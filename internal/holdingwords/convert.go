// Package holdingwords rewrites a dungeon file written in the content words
// rpg-project#1005 retired into the words that replaced them, changing only
// the renamed tokens and leaving every other byte of the file as it was.
//
// The map, the same one the engine's refusals name:
//
//   - `recognized:` (a line's requires, a post's pass, inside `not:`) is `holds:`;
//   - `recognize:` is `give:`, and inside it `topic:` is `holding:` and
//     `holder:` is `to:`;
//   - a `holds: <scalar>` whose value is a prop of the same file is
//     `carries:` (carrying a thing was the key's old meaning); a `holds:`
//     whose value is a list is an NPC's records and is left alone;
//   - a `holds:` whose value is a mapping with a `door` (a state's post) is
//     `post:`;
//   - a machine guard's fact `name: topic:<id>` is `name: holds.anyone:<id>`.
//
// The tokens are found by parsing, by the yaml node's line and column, never
// by searching the text: a line someone says that contains "recognized" is a
// value, not a key, and is never touched. An id that is both a prop and a
// holding something gives makes `holds: <id>` read two ways; it is left as
// written and reported, for the author to decide.
//
// A file already in the new words converts to itself, so running twice is
// safe.
package holdingwords

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Kind names one sort of rewrite, for the report.
type Kind string

// The rewrites, one per renamed word.
const (
	RecognizedToHolds   Kind = "recognized -> holds"
	RecognizeToGive     Kind = "recognize -> give"
	TopicToHolding      Kind = "topic -> holding (in give)"
	HolderToTo          Kind = "holder -> to (in give)"
	HoldsPropToCarries  Kind = "holds <prop> -> carries"
	StateHoldsToPost    Kind = "a state's holds -> post"
	GuardTopicToHoldsAn Kind = "guard topic:<id> -> holds.anyone:<id>"
)

// Change is one rewrite: where, which kind, and the text before and after.
type Change struct {
	Line   int
	Column int
	Kind   Kind
	From   string
	To     string
}

// Ambiguity is a token left as written because it reads two ways.
type Ambiguity struct {
	Line   int
	Column int
	Reason string
}

// ConvertInput is one dungeon file's source.
type ConvertInput struct {
	Source []byte
}

// ConvertOutput is what Convert did to one file.
type ConvertOutput struct {
	// Out is the converted file; equal to the input when Changes is empty.
	Out       []byte
	Changes   []Change
	Ambiguous []Ambiguity
}

// Counts is the number of changes of each kind.
func (r *ConvertOutput) Counts() map[Kind]int {
	out := make(map[Kind]int)
	for _, c := range r.Changes {
		out[c.Kind]++
	}
	return out
}

const guardTopicPrefix = "topic:"
const guardHoldsAnyonePrefix = "holds.anyone:"

// edit replaces the token at a node's position.
type edit struct {
	node *yaml.Node
	kind Kind
	from string // the text of the token now (a prefix of the value for a guard)
	to   string
}

// Convert rewrites raw's retired words. A file that is not YAML is an error;
// one with nothing to rewrite comes back unchanged with no Changes.
func Convert(in *ConvertInput) (*ConvertOutput, error) {
	if in == nil {
		return nil, errors.New("holdingwords: input is required")
	}
	raw := in.Source
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	c := &converter{props: map[string]bool{}, holdings: map[string]bool{}, seen: map[*yaml.Node]bool{}}
	c.collect(&doc, "")
	c.seen = map[*yaml.Node]bool{}
	c.walk(&doc)

	res := &ConvertOutput{Out: raw}
	if len(c.edits) == 0 {
		res.Ambiguous = c.ambiguous
		return res, nil
	}
	lines := lineStarts(raw)
	type placed struct {
		off int
		e   edit
	}
	ps := make([]placed, 0, len(c.edits))
	for _, e := range c.edits {
		off, err := tokenOffset(raw, lines, e.node, e.from)
		if err != nil {
			return nil, err
		}
		ps = append(ps, placed{off: off, e: e})
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].off > ps[j].off })
	out := bytes.Clone(raw)
	for i, p := range ps {
		if i > 0 && ps[i-1].off == p.off {
			continue // an alias reached the same token twice
		}
		out = append(out[:p.off], append([]byte(p.e.to), out[p.off+len(p.e.from):]...)...)
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].off < ps[j].off })
	for i, p := range ps {
		if i > 0 && ps[i-1].off == p.off {
			continue
		}
		res.Changes = append(res.Changes, Change{
			Line: p.e.node.Line, Column: p.e.node.Column, Kind: p.e.kind, From: p.e.from, To: p.e.to,
		})
	}
	res.Out = out
	res.Ambiguous = c.ambiguous
	return res, nil
}

type converter struct {
	props     map[string]bool
	holdings  map[string]bool
	edits     []edit
	ambiguous []Ambiguity
	seen      map[*yaml.Node]bool
}

// collect gathers the file's prop ids (a room object `kind: prop`, a key
// under `propBindings`) and every holding it names, in either spelling.
func (c *converter) collect(n *yaml.Node, parentKey string) {
	if n == nil || c.seen[n] {
		return
	}
	c.seen[n] = true
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, ch := range n.Content {
			c.collect(ch, parentKey)
		}
	case yaml.MappingNode:
		if v := mapValue(n, "kind"); v != nil && v.Value == "prop" {
			if id := mapValue(n, "id"); id != nil && id.Kind == yaml.ScalarNode {
				c.props[id.Value] = true
			}
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if parentKey == "propBindings" {
				c.props[k.Value] = true
			}
			switch k.Value {
			case "recognized":
				if v.Kind == yaml.ScalarNode {
					c.holdings[v.Value] = true
				}
			case "recognize", "give":
				for _, key := range []string{"topic", "holding"} {
					if h := mapValue(v, key); h != nil && h.Kind == yaml.ScalarNode {
						c.holdings[h.Value] = true
					}
				}
			case "name":
				if v.Kind == yaml.ScalarNode {
					for _, p := range []string{guardTopicPrefix, guardHoldsAnyonePrefix} {
						if strings.HasPrefix(v.Value, p) {
							c.holdings[strings.TrimPrefix(v.Value, p)] = true
						}
					}
				}
			}
			c.collect(v, k.Value)
		}
	case yaml.AliasNode:
		// The anchor is walked where it is defined.
	}
}

// walk records every rewrite.
func (c *converter) walk(n *yaml.Node) {
	if n == nil || c.seen[n] {
		return
	}
	c.seen[n] = true
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, ch := range n.Content {
			c.walk(ch)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			switch k.Value {
			case "recognized":
				c.rename(k, "holds", RecognizedToHolds)
			case "recognize":
				c.rename(k, "give", RecognizeToGive)
				c.renameInGive(v)
			case "give":
				c.renameInGive(v) // a half-converted give still loses its old inner words
			case "holds":
				c.holds(k, v)
			case "fact":
				if name := mapValue(v, "name"); name != nil && name.Kind == yaml.ScalarNode &&
					strings.HasPrefix(name.Value, guardTopicPrefix) {
					c.edits = append(c.edits, edit{node: name, kind: GuardTopicToHoldsAn,
						from: guardTopicPrefix, to: guardHoldsAnyonePrefix})
				}
			}
			c.walk(v)
		}
	}
}

func (c *converter) renameInGive(v *yaml.Node) {
	if v == nil || v.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(v.Content); i += 2 {
		switch v.Content[i].Value {
		case "topic":
			c.rename(v.Content[i], "holding", TopicToHolding)
		case "holder":
			c.rename(v.Content[i], "to", HolderToTo)
		}
	}
}

func (c *converter) holds(k, v *yaml.Node) {
	switch v.Kind {
	case yaml.ScalarNode:
		if !c.props[v.Value] {
			return // a holding, already the new word
		}
		if c.holdings[v.Value] {
			c.ambiguous = append(c.ambiguous, Ambiguity{Line: k.Line, Column: k.Column, Reason: fmt.Sprintf(
				"%q is both a prop and a holding, so `holds: %s` reads two ways: left as written "+
					"(write `carries: %s` for the prop, or rename the holding)", v.Value, v.Value, v.Value)})
			return
		}
		c.rename(k, "carries", HoldsPropToCarries)
	case yaml.MappingNode:
		if mapValue(v, "door") != nil {
			c.rename(k, "post", StateHoldsToPost)
		}
	}
}

func (c *converter) rename(k *yaml.Node, to string, kind Kind) {
	c.edits = append(c.edits, edit{node: k, kind: kind, from: k.Value, to: to})
}

// mapValue is the value under key in a mapping node, or nil.
func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// lineStarts is the byte offset of each line's start, 1-based by index-1.
func lineStarts(raw []byte) []int {
	starts := []int{0}
	for i, b := range raw {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// tokenOffset is the byte offset of the token `from` at the node: yaml's
// column counts characters, so it is walked as runes from the line's start,
// and a quoted scalar's text starts one past its quote. The bytes there must
// be the token, or the file is refused rather than edited blind.
func tokenOffset(raw []byte, lines []int, n *yaml.Node, from string) (int, error) {
	if n.Line < 1 || n.Line > len(lines) {
		return 0, fmt.Errorf("line %d: out of range", n.Line)
	}
	off := lines[n.Line-1]
	for col := 1; col < n.Column && off < len(raw); col++ {
		_, size := utf8.DecodeRune(raw[off:])
		off += size
	}
	if n.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle) != 0 {
		off++
	}
	if !bytes.HasPrefix(raw[off:], []byte(from)) {
		return 0, fmt.Errorf("line %d column %d: expected %q in the text, found something else; nothing was changed",
			n.Line, n.Column, from)
	}
	return off, nil
}
