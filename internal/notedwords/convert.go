// Package notedwords rewrites a dungeon file written in the content words
// rpg-project#1005 retired into the words that replaced them, changing only
// the renamed tokens and leaving every other byte of the file as it was.
//
// The map, the same one the engine's refusals name (encounter v0.222.1):
//
//   - `recognized: <topic>` (a line's requires, a post's pass, inside `not:`)
//     is `noted: <topic>`;
//   - `recognize:` is `give:`, and inside it `holder:` is `to:` (its `topic:`
//     stays);
//   - a `holds: <scalar>` whose value is a prop of the same file is
//     `carries:` (carrying a thing was the key's old meaning); a `holds:`
//     whose value is a list is an NPC's records and is left alone;
//   - a `holds:` whose value is a mapping with a `door` (a state's post) is
//     `post:`;
//   - a machine guard's fact `name: topic:<id>` is `name: noted.anyone:<id>`.
//
// It also converts the words that lived briefly in encounter v0.221.0 and
// v0.222.0 and were withdrawn: `holds: <topic>` (a value no prop has, that
// the file gives or asks as a topic) is `noted:`, `holding:` inside a give is
// `topic:`, and a guard's `holds:<id>` / `holds.anyone:<id>` is
// `noted:<id>` / `noted.anyone:<id>`.
//
// The tokens are found by parsing, by the yaml node's line and column, never
// by searching the text: a line someone says that contains "recognized" is a
// value, not a key, and is never touched. A `holds: <id>` is never guessed:
// an id that is both a prop and a topic reads two ways, and an id that is
// neither is not a word this converter knows; both are left as written and
// reported, for the author to decide.
//
// A file already in the new words converts to itself, so running twice is
// safe.
package notedwords

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
	RecognizedToNoted  Kind = "recognized -> noted"
	RecognizeToGive    Kind = "recognize -> give"
	HolderToTo         Kind = "holder -> to (in give)"
	HoldsPropToCarries Kind = "holds <prop> -> carries"
	StateHoldsToPost   Kind = "a state's holds -> post"
	GuardTopicToNoted  Kind = "guard topic:<id> -> noted.anyone:<id>"

	// The withdrawn v0.221.0/v0.222.0 words.
	HoldsTopicToNoted   Kind = "holds <topic> -> noted (withdrawn word)"
	HoldingToTopic      Kind = "holding -> topic (in give; withdrawn word)"
	GuardHoldsToNoted   Kind = "guard holds:<id> -> noted:<id> (withdrawn word)"
	GuardHoldsAnyToNote Kind = "guard holds.anyone:<id> -> noted.anyone:<id> (withdrawn word)"
)

// Change is one rewrite: where, which kind, and the text before and after.
type Change struct {
	Line   int
	Column int
	Kind   Kind
	From   string
	To     string
}

// Ambiguity is a token left as written because it cannot be read one way.
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

// guardPrefix is one guard-fact prefix rename. `holds.anyone:` is tried
// before `holds:`, which is not its prefix, so the order only reads well.
type guardPrefix struct {
	from, to string
	kind     Kind
}

// The guard-fact prefixes whose id is a topic, retired and current.
const (
	guardTopic       = "topic:"
	guardHoldsAnyone = "holds.anyone:"
	guardHolds       = "holds:"
	guardNotedAnyone = "noted.anyone:"
	guardNoted       = "noted:"
)

var guardPrefixes = []guardPrefix{
	{from: guardTopic, to: guardNotedAnyone, kind: GuardTopicToNoted},
	{from: guardHoldsAnyone, to: guardNotedAnyone, kind: GuardHoldsAnyToNote},
	{from: guardHolds, to: guardNoted, kind: GuardHoldsToNoted},
}

// topicGuardPrefixes are the guard-fact prefixes, old and new, whose id is a
// topic.
var topicGuardPrefixes = []string{guardTopic, guardHoldsAnyone, guardHolds, guardNotedAnyone, guardNoted}

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
		return nil, errors.New("notedwords: input is required")
	}
	raw := in.Source
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	c := &converter{props: map[string]bool{}, topics: map[string]bool{}, seen: map[*yaml.Node]bool{}}
	c.collect(&doc, "")
	c.seen = map[*yaml.Node]bool{}
	c.walk(&doc)

	res := &ConvertOutput{Out: raw, Ambiguous: c.ambiguous}
	if len(c.edits) == 0 {
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
	return res, nil
}

type converter struct {
	props     map[string]bool
	topics    map[string]bool
	edits     []edit
	ambiguous []Ambiguity
	seen      map[*yaml.Node]bool
}

// collect gathers the file's prop ids (a room object `kind: prop`, a key
// under `propBindings`) and every topic it gives or asks, in any spelling.
// A `holds: <scalar>` is not collected: whether it names a topic is the
// question this answers for it.
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
			case "recognized", "noted":
				if v.Kind == yaml.ScalarNode {
					c.topics[v.Value] = true
				}
			case "recognize", "give":
				for _, key := range []string{"topic", "holding"} {
					if t := mapValue(v, key); t != nil && t.Kind == yaml.ScalarNode {
						c.topics[t.Value] = true
					}
				}
			case "name":
				if v.Kind == yaml.ScalarNode {
					for _, p := range topicGuardPrefixes {
						if strings.HasPrefix(v.Value, p) {
							c.topics[strings.TrimPrefix(v.Value, p)] = true
							break
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
				c.rename(k, "noted", RecognizedToNoted)
			case "recognize":
				c.rename(k, "give", RecognizeToGive)
				c.renameInGive(v)
			case "give":
				c.renameInGive(v) // a half-converted give still loses its old inner words
			case "holds":
				c.holds(k, v)
			case "fact":
				c.guard(mapValue(v, "name"))
			}
			c.walk(v)
		}
	}
}

func (c *converter) guard(name *yaml.Node) {
	if name == nil || name.Kind != yaml.ScalarNode {
		return
	}
	for _, p := range guardPrefixes {
		if strings.HasPrefix(name.Value, p.from) {
			c.edits = append(c.edits, edit{node: name, kind: p.kind, from: p.from, to: p.to})
			return
		}
	}
}

func (c *converter) renameInGive(v *yaml.Node) {
	if v == nil || v.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(v.Content); i += 2 {
		switch v.Content[i].Value {
		case "holder":
			c.rename(v.Content[i], "to", HolderToTo)
		case "holding":
			c.rename(v.Content[i], "topic", HoldingToTopic)
		}
	}
}

func (c *converter) holds(k, v *yaml.Node) {
	switch v.Kind {
	case yaml.ScalarNode:
		prop, topic := c.props[v.Value], c.topics[v.Value]
		switch {
		case prop && topic:
			c.leave(k, fmt.Sprintf("%q is both a prop and a topic, so `holds: %s` reads two ways: left as written "+
				"(write `carries: %s` for the prop, or `noted: %s` for the topic)", v.Value, v.Value, v.Value, v.Value))
		case prop:
			c.rename(k, "carries", HoldsPropToCarries)
		case topic:
			c.rename(k, "noted", HoldsTopicToNoted)
		default:
			c.leave(k, fmt.Sprintf("%q is neither a prop nor a topic of this file, so `holds: %s` is left as written",
				v.Value, v.Value))
		}
	case yaml.MappingNode:
		if mapValue(v, "door") != nil {
			c.rename(k, "post", StateHoldsToPost)
		}
	}
}

func (c *converter) leave(k *yaml.Node, reason string) {
	c.ambiguous = append(c.ambiguous, Ambiguity{Line: k.Line, Column: k.Column, Reason: reason})
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
