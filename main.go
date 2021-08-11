package main

import (
	"context"
	"fmt"
	"time"

	g "github.com/AllenDang/giu"
	"github.com/AllenDang/imgui-go"
	"github.com/rs/zerolog/log"
)

type Logic func(...interface{}) (interface{}, error)

type Node struct {
	Id           int
	Name         string
	InLabels     []string
	InputPorts   []int
	InputPortChs []chan interface{}
	OutLabel     string
	OutputPort   []chan interface{}
	Logic        Logic
}

func logit(msg string, context *Node, err error) {
	logevent := log.Debug()
	if err != nil {
		logevent = log.Error().Err(err)
	}
	logevent.Msgf("%s(%d): %s\n", context.Name, context.Id, msg)
}

func (n *Node) Process(ctx context.Context) {
	var (
		err error
	)
	for {
		var (
			executionOutput interface{}
			executionArgs   []interface{}
		)
		select {
		case <-ctx.Done():
			return
		default:
			if n.InputPortChs != nil && len(n.InputPortChs) == len(n.InputPorts) {
				for _, inCh := range n.InputPortChs {
					val := <-inCh
					executionArgs = append(executionArgs, val)
				}
			}
			if len(n.InputPorts) == len(executionArgs) {
				executionOutput, err = n.Logic(executionArgs...)
				if err != nil {
					logit("Execution failed", n, err)
				}
				for _, ch := range n.OutputPort {
					ch <- executionOutput
				}
			}
		}
	}
}

func newNode(def NodeDefinition) *Node {
	node := Node{
		Id:           0,
		Name:         def.Name,
		InLabels:     def.InLabels,
		InputPorts:   make([]int, len(def.InLabels)),
		InputPortChs: make([]chan interface{}, 0),
		OutLabel:     def.OutputLabel,
		OutputPort:   make([]chan interface{}, 0),
		Logic:        def.Logic,
	}
	if len(node.InputPorts) == 0 {
		node.InputPortChs = nil
	}

	return &node
}

type NodeDefinition struct {
	Name        string
	InLabels    []string
	OutputLabel string
	Logic       Logic
}

var library []NodeDefinition

func defineNode(name string, inputs []string, outputLabel string, logic Logic) {
	if inputs == nil {
		inputs = make([]string, 0)
	}
	library = append(library, NodeDefinition{
		Name:        name,
		InLabels:    inputs,
		OutputLabel: outputLabel,
		Logic:       logic,
	})
}

func libraryDefinitions() {
	defineNode(
		"Time/now",
		nil,
		"now",
		func(args ...interface{}) (interface{}, error) {
			return time.Now().Second(), nil
		},
	)
	defineNode(
		"Math/add",
		[]string{"number 1", "number 2"},
		"result",
		func(args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("error executing node Math/add: Two arguments needed, got %d", len(args))
			}
			lhs := args[0].(int)
			rhs := args[1].(int)
			return lhs + rhs, nil
		},
	)
	defineNode(
		"Math/subtract",
		[]string{"number 1", "number 2"},
		"result",
		func(args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("error executing node Math/subtract: Two arguments needed, got %d", len(args))
			}
			lhs := args[0].(int)
			rhs := args[1].(int)
			return lhs - rhs, nil
		},
	)
	defineNode(
		"IO/println",
		[]string{"data"},
		"",
		func(args ...interface{}) (interface{}, error) {
			if len(args) > 0 {
				data := args[0]
				fmt.Println(data)
			}
			return nil, nil
		},
	)
}

func libraryMenu(id string, values []NodeDefinition, builder func(int, NodeDefinition) g.Widget) g.Layout {
	var layout g.Layout

	layout = append(layout, g.Custom(func() { imgui.PushID(id) }))

	if len(values) > 0 && builder != nil {
		for i, v := range values {
			valueRef := v
			widget := builder(i, valueRef)
			layout = append(layout, widget)
		}
	}

	layout = append(layout, g.Custom(func() { imgui.PopID() }))

	return layout
}

func loop() {
	g.SingleWindow().Layout(
		g.Custom(func() {
			if program.Running {
				g.Button("Stop").OnClick(program.Stop).Build()
			} else {
				g.Button("Start").OnClick(program.Run).Build()
			}

		}),
		g.Custom(func() {
			imgui.ImNodesBeginNodeEditor()
			g.PushWindowPadding(10, 10)
			g.Popup("library").Layout(
				libraryMenu("library-menu", library, func(i int, nd NodeDefinition) g.Widget {
					return g.Selectable(nd.Name).OnClick(func() {
						node := newNode(nd)
						program.AddNode(node)
						for i := range node.InputPorts {
							node.InputPorts[i] = program.NextID()
						}
						mousePos := g.GetMousePos()
						imgui.ImNodesSetNodeScreenSpacePos(node.Id, g.ToVec2(mousePos))
					})
				}),
			).Build()
			g.PopStyle()
			if g.IsMouseClicked(g.MouseButtonRight) {
				g.OpenPopup("library")
			}
			for _, node := range program.Nodes {
				imgui.ImNodesBeginNode(node.Id)
				imgui.ImNodesBeginNodeTitleBar()
				g.Label(node.Name).Build()
				imgui.ImNodesEndNodeTitleBar()
				for i, inLabel := range node.InLabels {
					imgui.ImNodesBeginInputAttribute(node.InputPorts[i])
					w := imgui.CalcTextSize(inLabel, false, 256).X
					imgui.PushItemWidth(float32(nodeWidth) - w)
					g.Label(inLabel).Build()
					imgui.PopItemWidth()
					imgui.ImNodesEndInputAttribute()
				}

				if len(node.OutLabel) > 0 {
					g.Spacing().Build()
					imgui.ImNodesBeginOutputAttribute(node.Id)
					w := imgui.CalcTextSize(node.OutLabel, false, 256)
					g.Row(
						g.Dummy(float32(nodeWidth)-w.X, w.Y/2),
						g.Label(node.OutLabel),
					).Build()
					imgui.ImNodesEndOutputAttribute()
				}

				imgui.ImNodesEndNode()
			}

			for _, edges := range program.Edges {
				for _, edge := range edges {
					imgui.ImNodesLink(edge.Id, edge.From, edge.To)
				}
			}
			imgui.ImNodesEndNodeEditor()

			fromId, outputPort, toId, inputPort, _, created := imgui.ImNodesIsLinkCreated()
			if created {
				producerNode := program.Node(int(fromId))
				consumerNode := program.Node(int(toId))
				ch := make(chan interface{})
				consumerNode.InputPortChs = append(consumerNode.InputPortChs, ch)
				producerNode.OutputPort = append(producerNode.OutputPort, ch)

				program.AddEdge(int(outputPort), int(inputPort))
			}
		}),
	)
}

type Edge struct {
	Id   int
	From int
	To   int
}

type Graph struct {
	ctx       context.Context
	cancel    context.CancelFunc
	IdCounter int
	Nodes     []*Node
	Edges     map[int][]*Edge
	Running   bool
}

func (g *Graph) Node(id int) *Node {
	for _, node := range g.Nodes {
		if node.Id == id {
			return node
		}
	}
	return nil
}

func (g *Graph) NextID() int {
	id := g.IdCounter
	g.IdCounter += 1
	return id
}

func (g *Graph) AddNode(n *Node) int {
	n.Id = g.IdCounter
	g.Nodes = append(g.Nodes, n)
	g.IdCounter += 1
	return n.Id
}

func (g *Graph) AddEdge(from, to int) int {
	edge := &Edge{
		Id:   g.IdCounter,
		From: from,
		To:   to,
	}

	g.Edges[edge.Id] = append(g.Edges[edge.Id], edge)
	g.IdCounter += 1
	return edge.Id
}

func (g *Graph) Run() {
	if g.Running {
		return
	}
	if len(g.Nodes) > 0 {
		g.ctx, g.cancel = context.WithCancel(context.Background())
		for _, node := range g.Nodes {
			go node.Process(g.ctx)
		}
		g.Running = !g.Running
	}
}

func (g *Graph) Stop() {
	g.cancel()
	g.Running = !g.Running
}

func newGraph() *Graph {
	return &Graph{
		IdCounter: 0,
		Nodes:     make([]*Node, 0),
		Edges:     make(map[int][]*Edge),
	}
}

var (
	version     = "0.0.0"
	release     = "pre-alpha"
	windowTitle = "ActionText %s-%s"

	nodeWidth = 160

	program *Graph
)

func main() {
	libraryDefinitions()
	program = newGraph()
	wnd := g.NewMasterWindow(fmt.Sprintf(windowTitle, release, version), 1024, 768, 0)
	wnd.Run(loop)
}
