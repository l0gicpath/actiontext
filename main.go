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

type NodeInputPortType int

func (typ NodeInputPortType) Zero() interface{} {
	switch typ {
	case NodeInputPortTypeString, NodeInputPortTypeText:
		return ""
	case NodeInputPortTypeFloat:
		return 0.0
	case NodeInputPortTypeInt:
		return 0
	default:
		return nil
	}
}

const (
	NodeInputPortTypeString NodeInputPortType = iota
	NodeInputPortTypeInt
	NodeInputPortTypeFloat
	NodeInputPortTypeText
)

type NodeInputPort struct {
	Id    int
	Label string
	Type  NodeInputPortType
	Value interface{}
	Ch    chan interface{}
}

type Node struct {
	Id                 int
	Name               string
	unlinkedPortsCount int
	InputPorts         []*NodeInputPort
	OutLabel           string
	OutputPort         []chan interface{}
	Logic              Logic
}

func (n *Node) AllPortsLinked() bool {
	return n.unlinkedPortsCount == 0
}

func logit(msg string, context *Node, err error) {
	logevent := log.Debug()
	if err != nil {
		logevent = log.Error().Err(err)
	}
	logevent.Msgf("%s(%d): %s\n", context.Name, context.Id, msg)
}
func (n *Node) LinkPort(id int, ch chan interface{}) {
	if port := n.Port(id); port != nil {
		port.Ch = ch
		n.unlinkedPortsCount--
	}
}
func (n *Node) Port(id int) *NodeInputPort {
	for _, port := range n.InputPorts {
		if port.Id == id {
			return port
		}
	}
	return nil
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
			if n.InputPorts != nil {
				for _, port := range n.InputPorts {
					val := port.Value
					if port.Ch != nil {
						val = <-port.Ch
					}
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
		Id:         program.NextID(),
		Name:       def.Name,
		OutLabel:   def.OutputLabel,
		OutputPort: make([]chan interface{}, 0),
		Logic:      def.Logic,
	}
	if def.Inputs != nil {
		node.InputPorts = make([]*NodeInputPort, 0)
		node.unlinkedPortsCount = len(def.Inputs)
		for label, typ := range def.Inputs {
			node.InputPorts = append(node.InputPorts, &NodeInputPort{
				Id:    program.NextID(),
				Label: label,
				Type:  typ,
				Value: typ.Zero(),
				Ch:    nil,
			})
		}
	}
	return &node
}

type NodeInputs map[string]NodeInputPortType

type NodeDefinition struct {
	Name        string
	Inputs      NodeInputs
	OutputLabel string
	Logic       Logic
}

var library []NodeDefinition

func defineNode(name string, inputs NodeInputs, outputLabel string, logic Logic) {
	library = append(library, NodeDefinition{
		Name:        name,
		Inputs:      inputs,
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
		NodeInputs{
			"number 1": NodeInputPortTypeInt,
			"number 2": NodeInputPortTypeInt,
		},
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
		NodeInputs{
			"number 1": NodeInputPortTypeInt,
			"number 2": NodeInputPortTypeInt,
		},
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
		NodeInputs{
			"data": NodeInputPortTypeString,
		},
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
				if node.InputPorts != nil {
					for _, inputPort := range node.InputPorts {
						imgui.ImNodesBeginInputAttribute(inputPort.Id)
						w := imgui.CalcTextSize(inputPort.Label, false, 256).X
						// imgui.PushItemWidth(float32(nodeWidth) - w)
						// imgui.PopItemWidth()
						g.Label(inputPort.Label).Build()
						if inputPort.Ch == nil {
							g.SameLine()
							switch inputPort.Type {
							case NodeInputPortTypeFloat:
								val := (inputPort.Value.(float32))
								g.InputFloat("##hidelabel", &val).Size(float32(nodeWidth) - w).Build()
							case NodeInputPortTypeInt:
								val := int32((inputPort.Value.(int)))
								g.InputInt(&val).OnChange(func() {
									inputPort.Value = int(val)
								}).Size(float32(nodeWidth) - w).Build()
							case NodeInputPortTypeString:
								val := (inputPort.Value.(string))
								g.InputText(&val).OnChange(func() {
									inputPort.Value = val
								}).Size(float32(nodeWidth) - w).Build()
							}
						}
						imgui.ImNodesEndInputAttribute()
					}
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
				consumerNode.LinkPort(int(inputPort), ch)
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
	g.Nodes = append(g.Nodes, n)
	return n.Id
}

func (g *Graph) AddEdge(from, to int) int {
	edge := &Edge{
		Id:   g.NextID(),
		From: from,
		To:   to,
	}

	g.Edges[edge.Id] = append(g.Edges[edge.Id], edge)
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
