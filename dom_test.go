package vecty

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/gopherjs/gopherjs/js"
)

var _ = func() bool {
	isTest = true
	return true
}()

type testCore struct{ Core }

func (testCore) Render() *HTML { return Tag("p") }

type testCorePtr struct{ *Core }

func (testCorePtr) Render() *HTML { return Tag("p") }

func TestCore(t *testing.T) {
	// Test that a standard *MyComponent with embedded Core works as we expect.
	t.Run("comp_ptr_and_core", func(t *testing.T) {
		v1 := Tag("v1")
		valid := Component(&testCore{})
		valid.Context().prevRender = v1
		if valid.Context().prevRender != v1 {
			t.Fatal("valid.Context().prevRender != v1")
		}
	})

	// Test that a non-pointer MyComponent with embedded Core does not satisfy
	// the Component interface:
	//
	//  testCore does not implement Component (Context method has pointer receiver)
	//
	t.Run("comp_and_core", func(t *testing.T) {
		isComponent := func(x interface{}) bool {
			_, ok := x.(Component)
			return ok
		}
		if isComponent(testCore{}) {
			t.Fatal("expected !isComponent(testCompCore{})")
		}
	})

	// Test what happens when a user accidentally embeds *Core instead of Core in
	// their component.
	t.Run("comp_ptr_and_core_ptr", func(t *testing.T) {
		v1 := Tag("v1")
		invalid := Component(&testCorePtr{})
		got := recoverStr(func() {
			invalid.Context().prevRender = v1
		})
		// TODO(slimsag): This would happen in user-facing code too. We should
		// create a helper for when we access a component's context, which
		// would panic with a more helpful message.
		want := "runtime error: invalid memory address or nil pointer dereference"
		if got != want {
			t.Fatalf("got panic %q want %q", got, want)
		}
	})
	t.Run("comp_and_core_ptr", func(t *testing.T) {
		v1 := Tag("v1")
		invalid := Component(testCorePtr{})
		got := recoverStr(func() {
			invalid.Context().prevRender = v1
		})
		// TODO(slimsag): This would happen in user-facing code too. We should
		// create a helper for when we access a component's context, which
		// would panic with a more helpful message.
		want := "runtime error: invalid memory address or nil pointer dereference"
		if got != want {
			t.Fatalf("got panic %q want %q", got, want)
		}
	})
}

// TODO(slimsag): TestUnmounter; Unmounter.Unmount

func TestHTML_Node(t *testing.T) {
	// Create a non-nil *js.Object. For 'gopherjs test', &js.Object{} == nil
	// because it is special-cased; but for 'go test' js.Global == nil.
	x := js.Global // used for 'gopherjs test'
	if x == nil {
		x = &js.Object{} // used for 'go test'
	}
	h := &HTML{node: wrapObject(x)}
	if h.Node() != x {
		t.Fatal("h.Node() != x")
	}
}

// TestHTML_reconcile_std tests that (*HTML).reconcile against an old HTML instance
// works as expected (i.e. that it updates nodes correctly).
func TestHTML_reconcile_std(t *testing.T) {
	t.Run("text_identical", func(t *testing.T) {
		h := Text("foobar")
		hNode := &mockObject{}
		h.node = hNode
		prev := Text("foobar")
		prevNode := &mockObject{}
		prev.node = prevNode
		h.reconcile(prev)
		if h.node != prevNode {
			t.Fatal("h.node != prevNode")
		}
	})
	t.Run("text_diff", func(t *testing.T) {
		want := "bar"
		h := Text(want)
		hNode := &mockObject{}
		h.node = hNode
		prev := Text("foo")
		setNodeValue := ""
		prevNode := &mockObject{
			set: func(key string, value interface{}) {
				if key != "nodeValue" {
					panic(`key != "nodeValue"`)
				}
				setNodeValue = value.(string)
			},
		}
		prev.node = prevNode
		h.reconcile(prev)
		if h.node != prevNode {
			t.Fatal("h.node != prevNode")
		}
		if setNodeValue != want {
			t.Fatalf("got %q want %q", setNodeValue, want)
		}
	})
	t.Run("properties", func(t *testing.T) {
		cases := []struct {
			name         string
			initHTML     *HTML
			initResult   string
			targetHTML   *HTML
			targetResult string
		}{
			{
				name:         "diff",
				initHTML:     Tag("div", Markup(Property("a", 1), Property("b", "2foobar"))),
				initResult:   "a:1 b:2foobar",
				targetHTML:   Tag("div", Markup(Property("a", 3), Property("b", "4foobar"))),
				targetResult: "a:3 b:4foobar",
			},
			{
				name:         "remove",
				initHTML:     Tag("div", Markup(Property("a", 1), Property("b", "2foobar"))),
				initResult:   "a:1 b:2foobar",
				targetHTML:   Tag("div", Markup(Property("a", 3))),
				targetResult: "a:3",
			},
			{
				name:         "replaced_elem_diff",
				initHTML:     Tag("div", Markup(Property("a", 1), Property("b", "2foobar"))),
				initResult:   "a:1 b:2foobar",
				targetHTML:   Tag("span", Markup(Property("a", 3), Property("b", "4foobar"))),
				targetResult: "a:3 b:4foobar",
			},
			{
				name:         "replaced_elem_shared",
				initHTML:     Tag("div", Markup(Property("a", 1), Property("b", "2foobar"))),
				initResult:   "a:1 b:2foobar",
				targetHTML:   Tag("span", Markup(Property("a", 1), Property("b", "4foobar"))),
				targetResult: "a:1 b:4foobar",
			},
		}
		for _, tst := range cases {
			t.Run(tst.name, func(t *testing.T) {
				initSet := make(map[string]interface{})
				targetSet := make(map[string]interface{})
				initElem := &mockObject{
					set: func(key string, value interface{}) {
						initSet[key] = value
					},
					delete: func(key string) {
						delete(initSet, key)
					},
				}
				targetElem := &mockObject{
					set: func(key string, value interface{}) {
						targetSet[key] = value
					},
					delete: func(key string) {
						delete(targetSet, key)
					},
				}
				wrapperFunc := func(obj jsObject) func(string, ...interface{}) jsObject {
					return func(name string, args ...interface{}) jsObject {
						if name != "createElement" {
							panic(fmt.Sprintf("expected call to createElement, not %q", name))
						}
						if len(args) != 1 {
							panic("len(args) != 1")
						}
						if args[0].(string) != "div" && args[0].(string) != "span" {
							panic(`args[0].(string) != "div|span"`)
						}
						return obj
					}
				}
				global = &mockObject{
					get: map[string]jsObject{
						"document": &mockObject{call: wrapperFunc(initElem)},
					},
				}
				tst.initHTML.reconcile(nil)
				got := sortedMapString(initSet)
				if got != tst.initResult {
					t.Fatalf("got %q want %q", got, tst.initResult)
				}
				matchingTags := tst.initHTML.tag == tst.targetHTML.tag
				if !matchingTags {
					global = &mockObject{
						get: map[string]jsObject{
							"document": &mockObject{call: wrapperFunc(targetElem)},
						},
					}
				}
				tst.targetHTML.reconcile(tst.initHTML)
				if matchingTags {
					got = sortedMapString(initSet)
				} else {
					got = sortedMapString(targetSet)
				}
				if got != tst.targetResult {
					t.Fatalf("got %q want %q", got, tst.targetResult)
				}
			})
		}
	})
	t.Run("attributes", func(t *testing.T) {
		cases := []struct {
			name         string
			initHTML     *HTML
			initResult   string
			targetHTML   *HTML
			targetResult string
		}{
			{
				name:         "diff",
				initHTML:     Tag("div", Markup(Attribute("a", 1), Attribute("b", "2foobar"))),
				initResult:   "a:1 b:2foobar",
				targetHTML:   Tag("div", Markup(Attribute("a", 3), Attribute("b", "4foobar"))),
				targetResult: "a:3 b:4foobar",
			},
			{
				name:         "remove",
				initHTML:     Tag("div", Markup(Attribute("a", 1), Attribute("b", "2foobar"))),
				initResult:   "a:1 b:2foobar",
				targetHTML:   Tag("div", Markup(Attribute("a", 3))),
				targetResult: "a:3",
			},
		}
		for _, tst := range cases {
			t.Run(tst.name, func(t *testing.T) {
				set := map[string]interface{}{}
				div := &mockObject{
					call: func(name string, args ...interface{}) jsObject {
						switch name {
						case "setAttribute":
							if len(args) != 2 {
								panic("setAttribute: len(args) != 2")
							}
							set[args[0].(string)] = args[1]
						case "removeAttribute":
							if len(args) != 1 {
								panic("removeAttribute: len(args) != 1")
							}
							delete(set, args[0].(string))
						default:
							panic(fmt.Sprintf("expected call to [setAttribute, removeAttribute], not %q", name))
						}
						return nil
					},
				}
				document := &mockObject{
					call: func(name string, args ...interface{}) jsObject {
						if name != "createElement" {
							panic(fmt.Sprintf("expected call to createElement, not %q", name))
						}
						if len(args) != 1 {
							panic("len(args) != 1")
						}
						if args[0].(string) != "div" {
							panic(`args[0].(string) != "div"`)
						}
						return div
					},
				}
				global = &mockObject{
					get: map[string]jsObject{
						"document": document,
					},
				}
				tst.initHTML.reconcile(nil)
				got := sortedMapString(set)
				if got != tst.initResult {
					t.Fatalf("got %q want %q", got, tst.initResult)
				}
				tst.targetHTML.reconcile(tst.initHTML)
				got = sortedMapString(set)
				if got != tst.targetResult {
					t.Fatalf("got %q want %q", got, tst.targetResult)
				}
			})
		}
	})
	t.Run("dataset", func(t *testing.T) {
		cases := []struct {
			name         string
			initHTML     *HTML
			initResult   string
			targetHTML   *HTML
			targetResult string
		}{
			{
				name:         "diff",
				initHTML:     Tag("div", Markup(Data("a", "1"), Data("b", "2foobar"))),
				initResult:   "a:1 b:2foobar",
				targetHTML:   Tag("div", Markup(Data("a", "3"), Data("b", "4foobar"))),
				targetResult: "a:3 b:4foobar",
			},
			{
				name:         "remove",
				initHTML:     Tag("div", Markup(Data("a", "1"), Data("b", "2foobar"))),
				initResult:   "a:1 b:2foobar",
				targetHTML:   Tag("div", Markup(Data("a", "3"))),
				targetResult: "a:3",
			},
		}
		for _, tst := range cases {
			t.Run(tst.name, func(t *testing.T) {
				set := map[string]interface{}{}
				dataset := &mockObject{
					set: func(key string, value interface{}) {
						set[key] = value
					},
					delete: func(key string) {
						delete(set, key)
					},
				}
				div := &mockObject{
					get: map[string]jsObject{
						"dataset": dataset,
					},
				}
				document := &mockObject{
					call: func(name string, args ...interface{}) jsObject {
						if name != "createElement" {
							panic(fmt.Sprintf("expected call to createElement, not %q", name))
						}
						if len(args) != 1 {
							panic("len(args) != 1")
						}
						if args[0].(string) != "div" {
							panic(`args[0].(string) != "div"`)
						}
						return div
					},
				}
				global = &mockObject{
					get: map[string]jsObject{
						"document": document,
					},
				}
				tst.initHTML.reconcile(nil)
				got := sortedMapString(set)
				if got != tst.initResult {
					t.Fatalf("got %q want %q", got, tst.initResult)
				}
				tst.targetHTML.reconcile(tst.initHTML)
				got = sortedMapString(set)
				if got != tst.targetResult {
					t.Fatalf("got %q want %q", got, tst.targetResult)
				}
			})
		}
	})
	t.Run("style", func(t *testing.T) {
		cases := []struct {
			name         string
			initHTML     *HTML
			initResult   string
			targetHTML   *HTML
			targetResult string
		}{
			{
				name:         "diff",
				initHTML:     Tag("div", Markup(Style("a", "1"), Style("b", "2foobar"))),
				initResult:   "a:1 b:2foobar",
				targetHTML:   Tag("div", Markup(Style("a", "3"), Style("b", "4foobar"))),
				targetResult: "a:3 b:4foobar",
			},
			{
				name:         "remove",
				initHTML:     Tag("div", Markup(Style("a", "1"), Style("b", "2foobar"))),
				initResult:   "a:1 b:2foobar",
				targetHTML:   Tag("div", Markup(Style("a", "3"))),
				targetResult: "a:3",
			},
		}
		for _, tst := range cases {
			t.Run(tst.name, func(t *testing.T) {
				set := map[string]interface{}{}
				style := &mockObject{
					call: func(name string, args ...interface{}) jsObject {
						switch name {
						case "setProperty":
							if len(args) != 2 {
								panic("setProperty: len(args) != 2")
							}
							set[args[0].(string)] = args[1]
						case "removeProperty":
							if len(args) != 1 {
								panic("removeProperty: len(args) != 1")
							}
							delete(set, args[0].(string))
						default:
							panic(fmt.Sprintf("expected call to [setProperty, removeProperty], not %q", name))
						}
						return nil
					},
				}
				div := &mockObject{
					get: map[string]jsObject{
						"style": style,
					},
				}
				document := &mockObject{
					call: func(name string, args ...interface{}) jsObject {
						if name != "createElement" {
							panic(fmt.Sprintf("expected call to createElement, not %q", name))
						}
						if len(args) != 1 {
							panic("len(args) != 1")
						}
						if args[0].(string) != "div" {
							panic(`args[0].(string) != "div"`)
						}
						return div
					},
				}
				global = &mockObject{
					get: map[string]jsObject{
						"document": document,
					},
				}
				tst.initHTML.reconcile(nil)
				got := sortedMapString(set)
				if got != tst.initResult {
					t.Fatalf("got %q want %q", got, tst.initResult)
				}
				tst.targetHTML.reconcile(tst.initHTML)
				got = sortedMapString(set)
				if got != tst.targetResult {
					t.Fatalf("got %q want %q", got, tst.targetResult)
				}
			})
		}
	})
	t.Run("event_listener", func(t *testing.T) {
		// TODO(pdf): Mock listener functions for equality testing
		cases := []struct {
			name                 string
			initEventListeners   []Applyer
			targetEventListeners []Applyer
		}{
			{
				name: "diff",
				initEventListeners: []Applyer{
					&EventListener{Name: "click"},
					&EventListener{Name: "keydown"},
				},
				targetEventListeners: []Applyer{
					&EventListener{Name: "click"},
				},
			},
		}
		for _, tst := range cases {
			t.Run(tst.name, func(t *testing.T) {
				addedListeners := map[string]func(*js.Object){}
				div := &mockObject{
					call: func(name string, args ...interface{}) jsObject {
						switch name {
						case "addEventListener":
							if len(args) != 2 {
								panic("addEventListener: len(args) != 2")
							}
							addedListeners[args[0].(string)] = args[1].(func(*js.Object))
						case "removeEventListener":
							if len(args) != 2 {
								panic("removeEventListener: len(args) != 2")
							}
							delete(addedListeners, args[0].(string))
						default:
							panic(fmt.Sprintf("unexpected call to %q", name))
						}
						return nil
					},
				}
				document := &mockObject{
					call: func(name string, args ...interface{}) jsObject {
						switch name {
						case "createElement":
							if len(args) != 1 {
								panic("len(args) != 1")
							}
							if args[0].(string) != "div" {
								panic(`args[0].(string) != "div"`)
							}
							return div
						default:
							panic(fmt.Sprintf("unexpected call to %q", name))
						}
					},
				}
				global = &mockObject{
					get: map[string]jsObject{
						"document": document,
					},
				}
				prev := Tag("div", Markup(tst.initEventListeners...))
				prev.reconcile(nil)
				for i, m := range tst.initEventListeners {
					listener := m.(*EventListener)
					if listener.wrapper == nil {
						t.Fatalf("listener %d wrapper == nil: %+v", i, listener)
					}
					if _, ok := addedListeners[listener.Name]; !ok {
						t.Fatalf("listener %d for %q not found: %+v", i, listener.Name, listener)
					}
				}
				if len(tst.initEventListeners) != len(addedListeners) {
					t.Fatalf("listener count mismatch: %d != %d", len(tst.initEventListeners), len(addedListeners))
				}
				h := Tag("div", Markup(tst.targetEventListeners...))
				h.reconcile(prev)
				for i, m := range tst.targetEventListeners {
					listener := m.(*EventListener)
					if listener.wrapper == nil {
						t.Fatalf("listener %d wrapper == nil: %+v", i, listener)
					}
					if _, ok := addedListeners[listener.Name]; !ok {
						t.Fatalf("listener %d for %q not found: %+v", i, listener.Name, listener)
					}
				}
				if len(tst.targetEventListeners) != len(addedListeners) {
					t.Fatalf("listener count mismatch: %d != %d", len(tst.targetEventListeners), len(addedListeners))
				}
			})
		}
	})

	// TODO(pdf): test (*HTML).reconcile child mutations, and value/checked properties
	// TODO(pdf): test multi-pass reconcile of persistent component pointer children, ref: https://github.com/gopherjs/vecty/pull/124
}

// TestHTML_reconcile_nil tests that (*HTML).reconcile(nil) works as expected (i.e.
// that it creates nodes correctly).
func TestHTML_reconcile_nil(t *testing.T) {
	t.Run("one_of_tag_or_text", func(t *testing.T) {
		got := recoverStr(func() {
			h := &HTML{text: "hello", tag: "div"}
			h.reconcile(nil)
		})
		want := "vecty: only one of HTML.tag or HTML.text may be set"
		if got != want {
			t.Fatalf("got panic %q want %q", got, want)
		}
	})
	t.Run("unsafe_text", func(t *testing.T) {
		got := recoverStr(func() {
			h := &HTML{text: "hello", innerHTML: "foobar"}
			h.reconcile(nil)
		})
		want := "vecty: only HTML may have UnsafeHTML attribute"
		if got != want {
			t.Fatalf("got panic %q want %q", got, want)
		}
	})
	t.Run("create_element", func(t *testing.T) {
		strong := &mockObject{}
		createdElement := ""
		document := &mockObject{
			call: func(name string, args ...interface{}) jsObject {
				if name != "createElement" {
					panic(fmt.Sprintf("expected call to createElement, not %q", name))
				}
				if len(args) != 1 {
					panic("len(args) != 1")
				}
				createdElement = args[0].(string)
				return strong
			},
		}
		global = &mockObject{
			get: map[string]jsObject{
				"document": document,
			},
		}
		want := "strong"
		h := Tag(want)
		h.reconcile(nil)
		if createdElement != want {
			t.Fatalf("createdElement %q want %q", createdElement, want)
		}
	})
	t.Run("create_element_ns", func(t *testing.T) {
		strong := &mockObject{}
		createdNamespace := ""
		createdElement := ""
		document := &mockObject{
			call: func(name string, args ...interface{}) jsObject {
				if name != "createElementNS" {
					panic(fmt.Sprintf("expected call to createElementNS, not %q", name))
				}
				if len(args) != 2 {
					panic("len(args) != 2")
				}
				createdNamespace = args[0].(string)
				createdElement = args[1].(string)
				return strong
			},
		}
		global = &mockObject{
			get: map[string]jsObject{
				"document": document,
			},
		}
		wantTag := "strong"
		wantNamespace := "foobar"
		h := Tag(wantTag, Markup(Namespace(wantNamespace)))
		h.reconcile(nil)
		if createdElement != wantTag {
			t.Fatalf("createdElement %q want tag %q", createdElement, wantTag)
		}
		if createdNamespace != wantNamespace {
			t.Fatalf("createdNamespace %q want namespace %q", createdElement, wantNamespace)
		}
	})
	t.Run("create_text_node", func(t *testing.T) {
		textNode := &mockObject{}
		createdTextNode := ""
		document := &mockObject{
			call: func(name string, args ...interface{}) jsObject {
				if name != "createTextNode" {
					panic(fmt.Sprintf("expected call to createTextNode, not %q", name))
				}
				if len(args) != 1 {
					panic("len(args) != 1")
				}
				createdTextNode = args[0].(string)
				return textNode
			},
		}
		global = &mockObject{
			get: map[string]jsObject{
				"document": document,
			},
		}
		want := "hello"
		h := &HTML{text: want}
		h.reconcile(nil)
		if createdTextNode != want {
			t.Fatalf("createdTextNode %q want %q", createdTextNode, want)
		}
	})
	t.Run("inner_html", func(t *testing.T) {
		setInnerHTML := ""
		div := &mockObject{
			set: func(key string, value interface{}) {
				if key != "innerHTML" {
					panic(fmt.Sprintf(`expected document.set "innerHTML", not %q`, key))
				}
				setInnerHTML = value.(string)
			},
		}
		document := &mockObject{
			call: func(name string, args ...interface{}) jsObject {
				if name != "createElement" {
					panic(fmt.Sprintf("expected call to createElement, not %q", name))
				}
				if len(args) != 1 {
					panic("len(args) != 1")
				}
				if args[0].(string) != "div" {
					panic(`args[0].(string) != "div"`)
				}
				return div
			},
		}
		global = &mockObject{
			get: map[string]jsObject{
				"document": document,
			},
		}
		want := "<p>hello</p>"
		h := Tag("div", Markup(UnsafeHTML(want)))
		h.reconcile(nil)
		if setInnerHTML != want {
			t.Fatalf("setInnerHTML %q want %q", setInnerHTML, want)
		}
	})
	t.Run("properties", func(t *testing.T) {
		set := map[string]interface{}{}
		div := &mockObject{
			set: func(key string, value interface{}) {
				set[key] = value
			},
		}
		document := &mockObject{
			call: func(name string, args ...interface{}) jsObject {
				if name != "createElement" {
					panic(fmt.Sprintf("expected call to createElement, not %q", name))
				}
				if len(args) != 1 {
					panic("len(args) != 1")
				}
				if args[0].(string) != "div" {
					panic(`args[0].(string) != "div"`)
				}
				return div
			},
		}
		global = &mockObject{
			get: map[string]jsObject{
				"document": document,
			},
		}
		h := Tag("div", Markup(Property("a", 1), Property("b", "2foobar")))
		h.reconcile(nil)
		got := sortedMapString(set)
		want := "a:1 b:2foobar"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
	t.Run("attributes", func(t *testing.T) {
		set := map[string]interface{}{}
		div := &mockObject{
			call: func(name string, args ...interface{}) jsObject {
				if name != "setAttribute" {
					panic(fmt.Sprintf("expected call to setAttribute, not %q", name))
				}
				if len(args) != 2 {
					panic("len(args) != 2")
				}
				set[args[0].(string)] = args[1]
				return nil
			},
		}
		document := &mockObject{
			call: func(name string, args ...interface{}) jsObject {
				if name != "createElement" {
					panic(fmt.Sprintf("expected call to createElement, not %q", name))
				}
				if len(args) != 1 {
					panic("len(args) != 1")
				}
				if args[0].(string) != "div" {
					panic(`args[0].(string) != "div"`)
				}
				return div
			},
		}
		global = &mockObject{
			get: map[string]jsObject{
				"document": document,
			},
		}
		h := Tag("div", Markup(Attribute("a", 1), Attribute("b", "2foobar")))
		h.reconcile(nil)
		got := sortedMapString(set)
		want := "a:1 b:2foobar"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
	t.Run("dataset", func(t *testing.T) {
		set := map[string]interface{}{}
		dataset := &mockObject{
			set: func(key string, value interface{}) {
				set[key] = value
			},
		}
		div := &mockObject{
			get: map[string]jsObject{
				"dataset": dataset,
			},
		}
		document := &mockObject{
			call: func(name string, args ...interface{}) jsObject {
				if name != "createElement" {
					panic(fmt.Sprintf("expected call to createElement, not %q", name))
				}
				if len(args) != 1 {
					panic("len(args) != 1")
				}
				if args[0].(string) != "div" {
					panic(`args[0].(string) != "div"`)
				}
				return div
			},
		}
		global = &mockObject{
			get: map[string]jsObject{
				"document": document,
			},
		}
		h := Tag("div", Markup(Data("a", "1"), Data("b", "2foobar")))
		h.reconcile(nil)
		got := sortedMapString(set)
		want := "a:1 b:2foobar"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
	t.Run("style", func(t *testing.T) {
		set := map[string]interface{}{}
		style := &mockObject{
			call: func(name string, args ...interface{}) jsObject {
				if name != "setProperty" {
					panic(fmt.Sprintf("expected call to setProperty, not %q", name))
				}
				if len(args) != 2 {
					panic("len(args) != 2")
				}
				set[args[0].(string)] = args[1]
				return nil
			},
		}
		div := &mockObject{
			get: map[string]jsObject{
				"style": style,
			},
		}
		document := &mockObject{
			call: func(name string, args ...interface{}) jsObject {
				if name != "createElement" {
					panic(fmt.Sprintf("expected call to createElement, not %q", name))
				}
				if len(args) != 1 {
					panic("len(args) != 1")
				}
				if args[0].(string) != "div" {
					panic(`args[0].(string) != "div"`)
				}
				return div
			},
		}
		global = &mockObject{
			get: map[string]jsObject{
				"document": document,
			},
		}
		h := Tag("div", Markup(Style("a", "1"), Style("b", "2foobar")))
		h.reconcile(nil)
		got := sortedMapString(set)
		want := "a:1 b:2foobar"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
	t.Run("add_event_listener", func(t *testing.T) {
		addedListeners := map[string]func(*js.Object){}
		div := &mockObject{
			call: func(name string, args ...interface{}) jsObject {
				switch name {
				case "addEventListener":
					if len(args) != 2 {
						panic("len(args) != 2")
					}
					addedListeners[args[0].(string)] = args[1].(func(*js.Object))
					return nil
				default:
					panic(fmt.Sprintf("unexpected call to %q", name))
				}
			},
		}
		document := &mockObject{
			call: func(name string, args ...interface{}) jsObject {
				switch name {
				case "createElement":
					if len(args) != 1 {
						panic("len(args) != 1")
					}
					if args[0].(string) != "div" {
						panic(`args[0].(string) != "div"`)
					}
					return div
				default:
					panic(fmt.Sprintf("unexpected call to %q", name))
				}
			},
		}
		global = &mockObject{
			get: map[string]jsObject{
				"document": document,
			},
		}
		e0 := &EventListener{Name: "click"}
		e1 := &EventListener{Name: "keydown"}
		h := Tag("div", Markup(e0, e1))
		h.reconcile(nil)
		if e0.wrapper == nil {
			t.Fatal("e0.wrapper == nil")
		}
		if e1.wrapper == nil {
			t.Fatal("e1.wrapper == nil")
		}
		if gotE0 := addedListeners["click"]; gotE0 == nil {
			t.Fatal("gotE0 == nil")
		}
		if gotE1 := addedListeners["keydown"]; gotE1 == nil {
			t.Fatal("gotE1 == nil")
		}
	})
	t.Run("children", func(t *testing.T) {
		var (
			divs    []jsObject
			appends = map[jsObject]jsObject{}
		)
		document := &mockObject{
			call: func(name string, args ...interface{}) jsObject {
				switch name {
				case "createElement":
					if len(args) != 1 {
						panic("len(args) != 1")
					}
					if args[0].(string) != "div" {
						panic(`args[0].(string) != "div"`)
					}
					div := &mockObject{}
					divs = append(divs, div)
					div.call = func(name string, args ...interface{}) jsObject {
						switch name {
						case "appendChild":
							if len(args) != 1 {
								panic("len(args) != 1")
							}
							appends[div] = args[0].(jsObject)
							return nil
						default:
							panic(fmt.Sprintf("unexpected call to %q", name))
						}
					}
					return div
				default:
					panic(fmt.Sprintf("unexpected call to %q", name))
				}
			},
		}
		global = &mockObject{
			get: map[string]jsObject{
				"document": document,
			},
		}
		var (
			compRenderCalls int
		)
		compRender := Tag("div")
		comp := &componentFunc{
			id: "foobar",
			render: func() *HTML {
				compRenderCalls++
				return compRender
			},
		}
		h := Tag("div", Tag("div", comp))
		h.reconcile(nil)
		if len(divs) != 3 {
			t.Fatal("len(divs) != 3")
		}
		if compRenderCalls != 1 {
			t.Fatal("compRenderCalls != 1")
		}
		if comp.Context().prevRenderComponent.(*componentFunc).id != comp.id {
			t.Fatal("comp.Context().prevRenderComponent.(*componentFunc).id != comp.id")
		}
		if comp.Context().prevRender != compRender {
			t.Fatal("comp.Context().prevRender != compRender")
		}
		root := divs[0]
		child := divs[1]
		child2 := divs[2]
		if appends[root] != child {
			t.Fatal("appends[root] != child")
		}
		if appends[child] != child2 {
			t.Fatal("appends[root] != child2")
		}
	})
	t.Run("children_render_nil", func(t *testing.T) {
		var (
			nodes   []jsObject
			appends = map[jsObject]jsObject{}
		)
		document := &mockObject{
			call: func(name string, args ...interface{}) jsObject {
				switch name {
				case "createElement":
					if len(args) != 1 {
						panic("len(args) != 1")
					}
					if n := args[0].(string); n != "div" && n != "noscript" {
						panic(`n != "div" && n != "noscript"`)
					}
					n := &mockObject{}
					nodes = append(nodes, n)
					n.call = func(name string, args ...interface{}) jsObject {
						switch name {
						case "appendChild":
							if len(args) != 1 {
								panic("len(args) != 1")
							}
							appends[n] = args[0].(jsObject)
							return nil
						default:
							panic(fmt.Sprintf("unexpected call to %q", name))
						}
					}
					return n
				default:
					panic(fmt.Sprintf("unexpected call to %q", name))
				}
			},
		}
		global = &mockObject{
			get: map[string]jsObject{
				"document": document,
			},
		}
		var (
			compRenderCalls int
		)
		comp := &componentFunc{
			id: "foobar",
			render: func() *HTML {
				compRenderCalls++
				return nil
			},
		}
		h := Tag("div", Tag("div", comp))
		h.reconcile(nil)
		if len(nodes) != 3 {
			t.Fatal("len(nodes) != 3")
		}
		if compRenderCalls != 1 {
			t.Fatal("compRenderCalls != 1")
		}
		if comp.Context().prevRenderComponent.(*componentFunc).id != comp.id {
			t.Fatal("comp.Context().prevRenderComponent.(*componentFunc).id != comp.id")
		}
		if comp.Context().prevRender == nil {
			t.Fatal("comp.Context().prevRender == nil")
		}
		root := nodes[0]
		child := nodes[1]
		child2 := nodes[2]
		if appends[root] != child {
			t.Fatal("appends[root] != child")
		}
		if appends[child] != child2 {
			t.Fatal("appends[root] != child2")
		}
	})
}

func TestTag(t *testing.T) {
	markupCalled := false
	want := "foobar"
	h := Tag(want, Markup(markupFunc(func(h *HTML) {
		markupCalled = true
	})))
	if !markupCalled {
		t.Fatal("expected markup to be applied")
	}
	if h.tag != want {
		t.Fatalf("got tag %q want tag %q", h.text, want)
	}
	if h.text != "" {
		t.Fatal("expected no text")
	}
}

func TestText(t *testing.T) {
	markupCalled := false
	want := "Hello world!"
	h := Text(want, Markup(markupFunc(func(h *HTML) {
		markupCalled = true
	})))
	if !markupCalled {
		t.Fatal("expected markup to be applied")
	}
	if h.text != want {
		t.Fatalf("got text %q want text %q", h.text, want)
	}
	if h.tag != "" {
		t.Fatal("expected no tag")
	}
}

// TestRerender_nil tests that Rerender panics when the component argument is
// nil.
func TestRerender_nil(t *testing.T) {
	gotPanic := ""
	func() {
		defer func() {
			r := recover()
			if r != nil {
				gotPanic = fmt.Sprint(r)
			}
		}()
		Rerender(nil)
	}()
	expected := "vecty: Rerender illegally called with a nil Component argument"
	if gotPanic != expected {
		t.Fatalf("got panic %q expected %q", gotPanic, expected)
	}
}

// TestRerender_no_prevRender tests the behavior of Rerender when there is no
// previous render.
func TestRerender_no_prevRender(t *testing.T) {
	var renderCallback func(float64)
	global = &mockObject{
		get: map[string]jsObject{
			"performance": &mockObject{
				call: func(name string, args ...interface{}) jsObject {
					if name != "now" {
						panic(fmt.Sprintf("expected call to now, not %q", name))
					}
					if len(args) != 0 {
						panic("len(args) != 0")
					}
					return &mockObject{floatValue: 0}
				},
			},
		},
		call: func(name string, args ...interface{}) jsObject {
			if name != "requestAnimationFrame" {
				panic(fmt.Sprintf("expected call to requestAnimationFrame, not %q", name))
			}
			if len(args) != 1 {
				panic("len(args) != 1")
			}
			var ok bool
			if renderCallback, ok = args[0].(func(float64)); !ok {
				panic("incorrect argument to requestAnimationFrame")
			}
			return &mockObject{intValue: 0}
		},
	}
	got := recoverStr(func() {
		Rerender(&componentFunc{
			render: func() *HTML {
				panic("expected no Render call")
			},
			skipRender: func(prev Component) bool {
				panic("expected no SkipRender call")
			},
		})
		renderCallback(0)
	})
	want := "vecty: Rerender invoked on Component that has never been rendered"
	if got != want {
		t.Fatalf("got panic %q expected %q", got, want)
	}
}

// TestRerender_identical tests the behavior of Rerender when there is a
// previous render which is identical to the new render.
func TestRerender_identical(t *testing.T) {
	// Perform the initial render of the component.
	body := &mockObject{}
	bodySet := false
	document := &mockObject{
		call: func(name string, args ...interface{}) jsObject {
			if name != "createElement" {
				panic(fmt.Sprintf("expected call to createElement, not %q", name))
			}
			if len(args) != 1 {
				panic("len(args) != 1")
			}
			if args[0].(string) != "body" {
				panic(`args[0].(string) != "body"`)
			}
			return body
		},
		get: map[string]jsObject{
			"readyState": &mockObject{stringValue: "complete"},
		},
		set: func(key string, value interface{}) {
			if key != "body" {
				panic(fmt.Sprintf(`expected document.set "body", not %q`, key))
			}
			if value != body {
				panic(fmt.Sprintf(`expected document.set body value, not %T %+v`, value, value))
			}
			bodySet = true
		},
	}
	var renderCallback func(float64)
	global = &mockObject{
		get: map[string]jsObject{
			"document": document,
			"performance": &mockObject{
				call: func(name string, args ...interface{}) jsObject {
					if name != "now" {
						panic(fmt.Sprintf("expected call to now, not %q", name))
					}
					if len(args) != 0 {
						panic("len(args) != 0")
					}
					return &mockObject{floatValue: 0}
				},
			},
		},
		call: func(name string, args ...interface{}) jsObject {
			if name != "requestAnimationFrame" {
				panic(fmt.Sprintf("expected call to requestAnimationFrame, not %q", name))
			}
			if len(args) != 1 {
				panic("len(args) != 1")
			}
			var ok bool
			if renderCallback, ok = args[0].(func(float64)); !ok {
				panic("incorrect argument to requestAnimationFrame")
			}
			return &mockObject{intValue: 0}
		},
	}

	render := Tag("body")
	var renderCalled, skipRenderCalled int
	comp := &componentFunc{
		id: "original",
		render: func() *HTML {
			renderCalled++
			return render
		},
	}
	RenderBody(comp)
	if !bodySet {
		t.Fatal("!bodySet")
	}
	if renderCalled != 1 {
		t.Fatal("renderCalled != 1")
	}
	if comp.Context().prevRender != render {
		t.Fatal("comp.Context().prevRender != render")
	}
	if comp.Context().prevRenderComponent.(*componentFunc).id != "original" {
		t.Fatal(`comp.Context().prevRenderComponent.(*componentFunc).id != "original"`)
	}

	// Perform a re-render.
	newRender := Tag("body")
	comp.id = "modified"
	comp.render = func() *HTML {
		renderCalled++
		return newRender
	}
	comp.skipRender = func(prev Component) bool {
		if comp.id != "modified" {
			panic(`comp.id != "modified"`)
		}
		if comp.Context().prevRenderComponent.(*componentFunc).id != "original" {
			panic(`comp.Context().prevRenderComponent.(*componentFunc).id != "original"`)
		}
		if prev.(*componentFunc).id != "original" {
			panic(`prev.(*componentFunc).id != "original"`)
		}
		skipRenderCalled++
		return false
	}
	Rerender(comp)
	renderCallback(0)
	global = nil // Expecting no JS calls past here
	if renderCalled != 2 {
		t.Fatal("renderCalled != 2")
	}
	if skipRenderCalled != 1 {
		t.Fatal("skipRenderCalled != 1")
	}
	if comp.Context().prevRender != newRender {
		t.Fatal("comp.Context().prevRender != newRender")
	}
	if comp.Context().prevRenderComponent.(*componentFunc).id != "modified" {
		t.Fatal(`comp.Context().prevRenderComponent.(*componentFunc).id != "modified"`)
	}
}

// TestRerender_change tests the behavior of Rerender when there is a
// previous render which is different from the new render.
func TestRerender_change(t *testing.T) {
	cases := []struct {
		name      string
		newRender *HTML
	}{
		{
			name:      "new_child",
			newRender: Tag("body", Tag("div")),
		},
		// TODO(slimsag): bug! nil produces <noscript> and we incorrectly try
		// to replace <body> with it! We should panic & warn the user.
		//{
		//	name:      "nil",
		//	newRender: nil,
		//},
	}
	for _, tst := range cases {
		t.Run(tst.name, func(t *testing.T) {
			// Perform the initial render of the component.
			var bodyAppendChild jsObject
			body := &mockObject{
				call: func(name string, args ...interface{}) jsObject {
					switch name {
					case "appendChild":
						if len(args) != 1 {
							panic("len(args) != 1")
						}
						bodyAppendChild = args[0].(jsObject)
						return nil
					default:
						panic(fmt.Sprintf("unexpected call to %q", name))
					}
				},
			}
			bodySet := false
			document := &mockObject{
				call: func(name string, args ...interface{}) jsObject {
					if name != "createElement" {
						panic(fmt.Sprintf("expected call to createElement, not %q", name))
					}
					if len(args) != 1 {
						panic("len(args) != 1")
					}
					if args[0].(string) != "body" {
						panic(`args[0].(string) != "body"`)
					}
					return body
				},
				get: map[string]jsObject{
					"readyState": &mockObject{stringValue: "complete"},
				},
				set: func(key string, value interface{}) {
					if key != "body" {
						panic(fmt.Sprintf(`expected document.set "body", not %q`, key))
					}
					if value != body {
						panic(fmt.Sprintf(`expected document.set body value, not %T %+v`, value, value))
					}
					bodySet = true
				},
			}
			var renderCallback func(float64)
			global = &mockObject{
				get: map[string]jsObject{
					"document": document,
					"performance": &mockObject{
						call: func(name string, args ...interface{}) jsObject {
							if name != "now" {
								panic(fmt.Sprintf("expected call to now, not %q", name))
							}
							if len(args) != 0 {
								panic("len(args) != 0")
							}
							return &mockObject{floatValue: 0}
						},
					},
				},
				call: func(name string, args ...interface{}) jsObject {
					if name != "requestAnimationFrame" {
						panic(fmt.Sprintf("expected call to requestAnimationFrame, not %q", name))
					}
					if len(args) != 1 {
						panic("len(args) != 1")
					}
					var ok bool
					if renderCallback, ok = args[0].(func(float64)); !ok {
						panic("incorrect argument to requestAnimationFrame")
					}
					return &mockObject{intValue: 0}
				},
			}

			render := Tag("body")
			var renderCalled, skipRenderCalled int
			comp := &componentFunc{
				id: "original",
				render: func() *HTML {
					renderCalled++
					return render
				},
			}
			RenderBody(comp)
			if !bodySet {
				t.Fatal("!bodySet")
			}
			if renderCalled != 1 {
				t.Fatal("renderCalled != 1")
			}
			if comp.Context().prevRender != render {
				t.Fatal("comp.Context().prevRender != render")
			}
			if comp.Context().prevRenderComponent.(*componentFunc).id != "original" {
				t.Fatal(`comp.Context().prevRenderComponent.(*componentFunc).id != "original"`)
			}

			// Perform a re-render.
			body = &mockObject{}
			newNode := &mockObject{}
			document = &mockObject{
				call: func(name string, args ...interface{}) jsObject {
					switch name {
					case "createElement":
						if len(args) != 1 {
							panic("len(args) != 1")
						}
						switch args[0].(string) {
						case "body":
							return body
						case "div", "noscript":
							return newNode
						default:
							panic("unexpected createElement call")
						}
					default:
						panic(fmt.Sprintf("unexpected call to %q", name))
					}
				},
			}
			global = &mockObject{
				get: map[string]jsObject{
					"document": document,
					"performance": &mockObject{
						call: func(name string, args ...interface{}) jsObject {
							if name != "now" {
								panic(fmt.Sprintf("expected call to now, not %q", name))
							}
							if len(args) != 0 {
								panic("len(args) != 0")
							}
							return &mockObject{floatValue: 0}
						},
					},
				},
				call: func(name string, args ...interface{}) jsObject {
					if name != "requestAnimationFrame" {
						panic(fmt.Sprintf("expected call to requestAnimationFrame, not %q", name))
					}
					if len(args) != 1 {
						panic("len(args) != 1")
					}
					var ok bool
					if renderCallback, ok = args[0].(func(float64)); !ok {
						panic("incorrect argument to requestAnimationFrame")
					}
					return &mockObject{intValue: 0}
				},
			}

			comp.id = "modified"
			comp.render = func() *HTML {
				renderCalled++
				return tst.newRender
			}
			comp.skipRender = func(prev Component) bool {
				if comp.id != "modified" {
					panic(`comp.id != "modified"`)
				}
				if comp.Context().prevRenderComponent.(*componentFunc).id != "original" {
					panic(`comp.Context().prevRenderComponent.(*componentFunc).id != "original"`)
				}
				if prev.(*componentFunc).id != "original" {
					panic(`prev.(*componentFunc).id != "original"`)
				}
				skipRenderCalled++
				return false
			}
			Rerender(comp)
			renderCallback(0)
			if renderCalled != 2 {
				t.Fatal("renderCalled != 2")
			}
			if skipRenderCalled != 1 {
				t.Fatal("skipRenderCalled != 1")
			}
			if comp.Context().prevRender != tst.newRender {
				t.Fatal("comp.Context().prevRender != tst.newRender")
			}
			if comp.Context().prevRenderComponent.(*componentFunc).id != "modified" {
				t.Fatal(`comp.Context().prevRenderComponent.(*componentFunc).id != "modified"`)
			}
			if bodyAppendChild != newNode {
				t.Fatal("bodyAppendChild != newNode")
			}
		})
	}
}

// TestRenderBody_ExpectsBody tests that RenderBody panics when something other
// than a "body" tag is rendered by the component.
func TestRenderBody_ExpectsBody(t *testing.T) {
	cases := []struct {
		name      string
		render    *HTML
		wantPanic string
	}{
		{
			name:      "text",
			render:    Text("Hello world!"),
			wantPanic: "vecty: RenderBody expected Component.Render to return a body tag, found \"\"", // TODO(slimsag): error message bug
		},
		{
			name:      "div",
			render:    Tag("div"),
			wantPanic: "vecty: RenderBody expected Component.Render to return a body tag, found \"div\"",
		},
		{
			name:      "nil",
			render:    nil,
			wantPanic: "vecty: RenderBody expected Component.Render to return a body tag, found \"noscript\"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			document := &mockObject{
				call: func(name string, args ...interface{}) jsObject {
					switch name {
					case "createElement", "createTextNode":
						if len(args) != 1 {
							panic("len(args) != 1")
						}
						return &mockObject{}
					default:
						panic(fmt.Sprintf("unexpected call to %q", name))
					}
				},
			}
			global = &mockObject{
				get: map[string]jsObject{
					"document": document,
					"performance": &mockObject{
						call: func(name string, args ...interface{}) jsObject {
							if name != "now" {
								panic(fmt.Sprintf("expected call to now, not %q", name))
							}
							if len(args) != 0 {
								panic("len(args) != 0")
							}
							return &mockObject{floatValue: 0}
						},
					},
				},
				call: func(name string, args ...interface{}) jsObject {
					if name != "requestAnimationFrame" {
						panic(fmt.Sprintf("expected call to requestAnimationFrame, not %q", name))
					}
					if len(args) != 1 {
						panic("len(args) != 1")
					}
					if _, ok := args[0].(func(float64)); !ok {
						panic("incorrect argument to requestAnimationFrame")
					}
					return &mockObject{intValue: 0}
				},
			}
			var gotPanic string
			func() {
				defer func() {
					r := recover()
					if r != nil {
						gotPanic = fmt.Sprint(r)
					}
				}()
				RenderBody(&componentFunc{
					render: func() *HTML {
						return c.render
					},
					skipRender: func(prev Component) bool { return false },
				})
			}()
			if c.wantPanic != gotPanic {
				t.Fatalf("want panic %q got panic %q", c.wantPanic, gotPanic)
			}
		})
	}
}

// TestRenderBody_RenderSkipper_Skip tests that RenderBody panics when the
// component's SkipRender method returns skip == true.
func TestRenderBody_RenderSkipper_Skip(t *testing.T) {
	body := &mockObject{}
	document := &mockObject{
		call: func(name string, args ...interface{}) jsObject {
			if name != "createElement" {
				panic(fmt.Sprintf("expected call to createElement, not %q", name))
			}
			if len(args) != 1 {
				panic("len(args) != 1")
			}
			if args[0].(string) != "body" {
				panic(`args[0].(string) != "body"`)
			}
			return body
		},
		get: map[string]jsObject{
			"readyState": &mockObject{stringValue: "complete"},
		},
		set: func(key string, value interface{}) {
			if key != "body" {
				panic(fmt.Sprintf(`expected document.set "body", not %q`, key))
			}
			if value != body {
				panic(fmt.Sprintf(`expected document.set body value, not %T %+v`, value, value))
			}
		},
	}
	global = &mockObject{
		get: map[string]jsObject{
			"document": document,
			"performance": &mockObject{
				call: func(name string, args ...interface{}) jsObject {
					if name != "now" {
						panic(fmt.Sprintf("expected call to now, not %q", name))
					}
					if len(args) != 0 {
						panic("len(args) != 0")
					}
					return &mockObject{floatValue: 0}
				},
			},
		},
		call: func(name string, args ...interface{}) jsObject {
			if name != "requestAnimationFrame" {
				panic(fmt.Sprintf("expected call to requestAnimationFrame, not %q", name))
			}
			if len(args) != 1 {
				panic("len(args) != 1")
			}
			if _, ok := args[0].(func(float64)); !ok {
				panic("incorrect argument to requestAnimationFrame")
			}
			return &mockObject{intValue: 0}
		},
	}
	comp := &componentFunc{
		render: func() *HTML {
			return Tag("body")
		},
		skipRender: func(prev Component) bool {
			return true
		},
	}
	fakePrevRender := *comp
	comp.Context().prevRenderComponent = &fakePrevRender
	got := recoverStr(func() {
		RenderBody(comp)
	})
	want := "vecty: RenderBody Component.SkipRender returned true"
	if got != want {
		t.Fatalf("got panic %q want %q", got, want)
	}
}

// TestRenderBody_Standard_loaded tests that RenderBody properly handles the
// standard case of rendering into the "body" tag when the DOM is in a loaded
// state.
func TestRenderBody_Standard_loaded(t *testing.T) {
	body := &mockObject{}
	bodySet := false
	document := &mockObject{
		call: func(name string, args ...interface{}) jsObject {
			if name != "createElement" {
				panic(fmt.Sprintf("expected call to createElement, not %q", name))
			}
			if len(args) != 1 {
				panic("len(args) != 1")
			}
			if args[0].(string) != "body" {
				panic(`args[0].(string) != "body"`)
			}
			return body
		},
		get: map[string]jsObject{
			"readyState": &mockObject{stringValue: "complete"},
		},
		set: func(key string, value interface{}) {
			if key != "body" {
				panic(fmt.Sprintf(`expected document.set "body", not %q`, key))
			}
			if value != body {
				panic(fmt.Sprintf(`expected document.set body value, not %T %+v`, value, value))
			}
			bodySet = true
		},
	}
	global = &mockObject{
		get: map[string]jsObject{
			"document": document,
			"performance": &mockObject{
				call: func(name string, args ...interface{}) jsObject {
					if name != "now" {
						panic(fmt.Sprintf("expected call to now, not %q", name))
					}
					if len(args) != 0 {
						panic("len(args) != 0")
					}
					return &mockObject{floatValue: 0}
				},
			},
		},
		call: func(name string, args ...interface{}) jsObject {
			if name != "requestAnimationFrame" {
				panic(fmt.Sprintf("expected call to requestAnimationFrame, not %q", name))
			}
			if len(args) != 1 {
				panic("len(args) != 1")
			}
			if _, ok := args[0].(func(float64)); !ok {
				panic("incorrect argument to requestAnimationFrame")
			}
			return &mockObject{intValue: 0}
		},
	}
	RenderBody(&componentFunc{
		render: func() *HTML {
			return Tag("body")
		},
	})
	if !bodySet {
		t.Fatalf("expected document.body to be set")
	}
}

// TestRenderBody_Standard_loading tests that RenderBody properly handles the
// standard case of rendering into the "body" tag when the DOM is in a loading
// state.
func TestRenderBody_Standard_loading(t *testing.T) {
	body := &mockObject{}
	bodySet := false
	var domLoadedEventListener func()
	document := &mockObject{
		call: func(name string, args ...interface{}) jsObject {
			switch name {
			case "createElement":
				if len(args) != 1 {
					panic("len(args) != 1")
				}
				if args[0].(string) != "body" {
					panic(`args[0].(string) != "body"`)
				}
				return body
			case "addEventListener":
				if len(args) != 2 {
					panic("len(args) != 2")
				}
				if args[0].(string) != "DOMContentLoaded" {
					panic(`args[0].(string) != "DOMContentLoaded"`)
				}
				domLoadedEventListener = args[1].(func())
				return nil
			default:
				panic(fmt.Sprintf("unexpected call to %q", name))
			}
		},
		get: map[string]jsObject{
			"readyState": &mockObject{stringValue: "loading"},
		},
		set: func(key string, value interface{}) {
			if key != "body" {
				panic(fmt.Sprintf(`expected document.set "body", not %q`, key))
			}
			if value != body {
				panic(fmt.Sprintf(`expected document.set body value, not %T %+v`, value, value))
			}
			bodySet = true
		},
	}
	global = &mockObject{
		get: map[string]jsObject{
			"document": document,
			"performance": &mockObject{
				call: func(name string, args ...interface{}) jsObject {
					if name != "now" {
						panic(fmt.Sprintf("expected call to now, not %q", name))
					}
					if len(args) != 0 {
						panic("len(args) != 0")
					}
					return &mockObject{floatValue: 0}
				},
			},
		},
		call: func(name string, args ...interface{}) jsObject {
			if name != "requestAnimationFrame" {
				panic(fmt.Sprintf("expected call to requestAnimationFrame, not %q", name))
			}
			if len(args) != 1 {
				panic("len(args) != 1")
			}
			if _, ok := args[0].(func(float64)); !ok {
				panic("incorrect argument to requestAnimationFrame")
			}
			return &mockObject{intValue: 0}
		},
	}
	RenderBody(&componentFunc{
		render: func() *HTML {
			return Tag("body")
		},
	})
	if domLoadedEventListener == nil {
		t.Fatalf("domLoadedEventListener == nil")
	}
	if bodySet {
		t.Fatalf("expected document.body to NOT be set")
	}
	domLoadedEventListener()
	if !bodySet {
		t.Fatalf("expected document.body to be set")
	}
}

func TestSetTitle(t *testing.T) {
	titleSet := ""
	document := &mockObject{
		set: func(key string, value interface{}) {
			if key != "title" {
				panic(fmt.Sprintf(`expected document.set "title", not %q`, key))
			}
			titleSet = value.(string)
		},
	}
	global = &mockObject{
		get: map[string]jsObject{
			"document": document,
		},
	}
	want := "foobar"
	SetTitle(want)
	if titleSet != want {
		t.Fatalf("titleSet is %q, want %q", titleSet, want)
	}
}

func TestAddStylesheet(t *testing.T) {
	linkSet := map[string]interface{}{}
	link := &mockObject{
		set: func(key string, value interface{}) {
			linkSet[key] = value
		},
	}
	appendedToHead := false
	head := &mockObject{
		call: func(name string, args ...interface{}) jsObject {
			switch name {
			case "appendChild":
				if len(args) != 1 {
					panic("len(args) != 1")
				}
				if args[0] != link {
					panic(`args[0] != link`)
				}
				appendedToHead = true
				return nil
			default:
				panic(fmt.Sprintf("unexpected call to %q", name))
			}
		},
	}
	document := &mockObject{
		call: func(name string, args ...interface{}) jsObject {
			switch name {
			case "createElement":
				if len(args) != 1 {
					panic("len(args) != 1")
				}
				if args[0].(string) != "link" {
					panic(`args[0].(string) != "link"`)
				}
				return link
			default:
				panic(fmt.Sprintf("unexpected call to %q", name))
			}
		},
		set: func(key string, value interface{}) {
			if key != "title" {
				panic(fmt.Sprintf(`expected document.set "title", not %q`, key))
			}
		},
		get: map[string]jsObject{
			"head": head,
		},
	}
	global = &mockObject{
		get: map[string]jsObject{
			"document": document,
		},
	}
	url := "https://google.com/foobar.css"
	AddStylesheet(url)
	if !appendedToHead {
		t.Fatal("expected link to be appended to document.head")
	}
	if linkSet["rel"] != "stylesheet" {
		t.Fatal(`linkSet["rel"] != "stylesheet"`)
	}
	if linkSet["href"] != url {
		t.Fatal(`linkSet["href"] != url`)
	}
}

// sortedMapString returns the map converted to a string, but sorted.
func sortedMapString(m map[string]interface{}) string {
	var strs []string
	for k, v := range m {
		strs = append(strs, fmt.Sprintf("%v:%v", k, v))
	}
	sort.Strings(strs)
	return strings.Join(strs, " ")
}

// recoverStr runs f and returns the recovered panic as a string.
func recoverStr(f func()) (s string) {
	defer func() {
		s = fmt.Sprint(recover())
	}()
	f()
	return
}

type componentFunc struct {
	Core
	id         string
	render     func() *HTML
	skipRender func(prev Component) bool
}

func (c *componentFunc) Render() *HTML                  { return c.render() }
func (c *componentFunc) SkipRender(prev Component) bool { return c.skipRender(prev) }

type mockObject struct {
	set         func(key string, value interface{})
	get         map[string]jsObject
	delete      func(key string)
	call        func(name string, args ...interface{}) jsObject
	stringValue string
	boolValue   bool
	intValue    int
	floatValue  float64
}

func (w *mockObject) Set(key string, value interface{})              { w.set(key, value) }
func (w *mockObject) Get(key string) jsObject                        { return w.get[key] }
func (w *mockObject) Delete(key string)                              { w.delete(key) }
func (w *mockObject) Call(name string, args ...interface{}) jsObject { return w.call(name, args...) }
func (w *mockObject) String() string                                 { return w.stringValue }
func (w *mockObject) Bool() bool                                     { return w.boolValue }
func (w *mockObject) Int() int                                       { return w.intValue }
func (w *mockObject) Float() float64                                 { return w.floatValue }
