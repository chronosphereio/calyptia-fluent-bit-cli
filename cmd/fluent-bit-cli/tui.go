package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	fluentbit "github.com/calyptia/fluent-bit-cli"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/guptarohit/asciigraph"
	"github.com/jedib0t/go-pretty/v6/table"
	"golang.org/x/term"
)

type model struct {
	ctx           context.Context
	textInput     textinput.Model
	fluentbit     *fluentbit.Client
	indexes       int
	selectedIndex int
	history       []fluentbit.Metrics
	infoLoaded    bool
	info          fluentbit.BuildInfo
	err           error
}

func initialModel(ctx context.Context) model {
	ti := textinput.NewModel()
	ti.Placeholder = "Fluent Bit Base URL"
	ti.Width = 40
	ti.SetValue("http://localhost:2020")
	ti.Focus()
	return model{
		ctx:       ctx,
		textInput: ti,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

type fetchBuildInfoMsg struct{}

func fetchBuildInfoCmd() tea.Cmd {
	return func() tea.Msg {
		return fetchBuildInfoMsg{}
	}
}

type fetchMetricsMsg struct{}

func fetchMetricsCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return fetchMetricsMsg{}
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			if m.fluentbit == nil {
				baseURL := m.textInput.Value()
				if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
					m.err = errors.New("invalid URL")
					return m, textinput.Blink
				}

				m.fluentbit = &fluentbit.Client{
					HTTPClient: http.DefaultClient,
					BaseURL:    strings.TrimSuffix(baseURL, "/"),
				}
				return m, tea.Batch(fetchBuildInfoCmd(), fetchMetricsCmd())
			}
		case tea.KeyShiftTab, tea.KeyLeft, tea.KeyUp:
			m.selectedIndex--
			if m.selectedIndex == -1 {
				m.selectedIndex = 0
			}
			return m, nil
		case tea.KeyTab, tea.KeyRight, tea.KeyDown:
			m.selectedIndex++
			if m.selectedIndex == m.indexes {
				m.selectedIndex = m.indexes - 1
				if m.selectedIndex == -1 {
					m.selectedIndex = 0
				}
			}
			return m, nil
		}
	case fetchBuildInfoMsg:
		info, err := m.fluentbit.BuildInfo(m.ctx)
		if err != nil {
			m.err = err
			return m, nil
		}

		m.infoLoaded = true
		m.info = info
		m.err = nil
		return m, nil
	case fetchMetricsMsg:
		return func() (tea.Model, tea.Cmd) {
			ctx, cancel := context.WithTimeout(m.ctx, time.Second+(time.Millisecond*200))
			defer cancel()

			mm, err := m.fluentbit.Metrics(ctx)
			if err != nil {
				m.err = err
				return m, fetchMetricsCmd()
			}

			m.indexes = len(mm.Input) + len(mm.Output)
			m.history = append(m.history, mm)
			m.err = nil
			return m, fetchMetricsCmd()
		}()
	}

	if m.fluentbit == nil {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) View() string {
	var doc strings.Builder

	if m.fluentbit == nil {
		doc.WriteString(
			"Fluent Bit Base URL\n" +
				m.textInput.View() + "\n",
		)
	}

	var selectedType, selectedMetric string
	inputNames := currentInputNames(m.history)
	outputNames := currentOutputNames(m.history)
	lenInputs := len(inputNames)
	lenOutputs := len(outputNames)
	if lenInputs != 0 && m.selectedIndex >= 0 && m.selectedIndex < lenInputs {
		selectedType = "input"
		selectedMetric = inputNames[m.selectedIndex]
	} else if lenOutputs != 0 && m.selectedIndex >= 0 && m.selectedIndex < (lenInputs+lenOutputs) {
		selectedType = "output"
		selectedMetric = outputNames[m.selectedIndex-lenInputs]
	}

	screenWidth, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		screenWidth = 80
	}

	if m.infoLoaded {
		msg := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Render(
			fmt.Sprintf("Fluent Bit version=%s edition=%s", m.info.FluentBit.Version, m.info.FluentBit.Edition),
		)
		doc.WriteString(msg + "\n")
	}

	if len(m.history) >= 2 {
		switch selectedType {
		case "input":
			input := normalizeInput(m.history, selectedMetric)
			doc.WriteString(
				lipgloss.JoinHorizontal(lipgloss.Bottom,
					renderPlot(renderPlotProps{
						Caption: "input " + selectedMetric + " records",
						Series:  input.RecordDeltas,
						Width:   screenWidth / 2,
						Height:  12,
					}),
					renderPlot(renderPlotProps{
						Caption: "input " + selectedMetric + " bytes",
						Series:  input.ByteDeltas,
						Width:   screenWidth / 2,
						Height:  12,
					}),
				) + "\n",
			)
		case "output":
			output := normalizeOutput(m.history, selectedMetric)
			doc.WriteString(
				lipgloss.JoinVertical(lipgloss.Left,
					lipgloss.JoinHorizontal(lipgloss.Bottom,
						renderPlot(renderPlotProps{
							Caption: "output " + selectedMetric + " proc_records",
							Series:  output.ProcRecordDeltas,
							Width:   screenWidth / 2,
							Height:  12,
						}),
						renderPlot(renderPlotProps{
							Caption: "output " + selectedMetric + " proc_bytes",
							Series:  output.ProcByteDeltas,
							Width:   screenWidth / 2,
							Height:  12,
						}),
					),
					lipgloss.JoinHorizontal(lipgloss.Bottom,
						renderPlot(renderPlotProps{
							Caption: "output " + selectedMetric + " errors",
							Series:  output.ErrorDeltas,
							Width:   screenWidth / 3,
							Height:  7,
						}),
						renderPlot(renderPlotProps{
							Caption: "output " + selectedMetric + " retries",
							Series:  output.RetryDeltas,
							Width:   screenWidth / 3,
							Height:  7,
						}),
						renderPlot(renderPlotProps{
							Caption: "output " + selectedMetric + " retries_failed",
							Series:  output.RetryFailedDelta,
							Width:   screenWidth / 3,
							Height:  7,
						}),
					),
				) + "\n",
			)
		}

		if lenInputs != 0 {
			rows := [][]string{{
				"name",
				"records",
				"bytes",
			}}
			for _, name := range inputNames {
				style := lipgloss.NewStyle()
				if selectedType == "input" && selectedMetric == name {
					style = style.Bold(true).Inline(true).Foreground(lipgloss.Color("11"))
				}

				delta := currentInputDelta(m.history, name)
				rows = append(rows, []string{
					style.Render(name),
					style.Render(fmt.Sprintf("%.2f", delta.RecordDeltas)),
					style.Render(fmt.Sprintf("%.2f", delta.ByteDeltas)),
				})
			}
			doc.WriteString(
				renderTable("Input", rows) + "\n",
			)
		}

		if lenOutputs != 0 {
			rows := [][]string{{
				"name",
				"proc_records",
				"proc_bytes",
				"errors",
				"retries",
				"retries_failed",
			}}
			for _, name := range outputNames {
				style := lipgloss.NewStyle()
				if selectedType == "output" && selectedMetric == name {
					style = style.Bold(true).Inline(true).Foreground(lipgloss.Color("11"))
				}

				delta := currentOutputDelta(m.history, name)
				rows = append(rows, []string{
					style.Render(name),
					style.Render(fmt.Sprintf("%.2f", delta.ProcRecordDeltas)),
					style.Render(fmt.Sprintf("%.2f", delta.ProcByteDeltas)),
					style.Render(fmt.Sprintf("%.2f", delta.ErrorDeltas)),
					style.Render(fmt.Sprintf("%.2f", delta.RetryDeltas)),
					style.Render(fmt.Sprintf("%.2f", delta.RetryFailedDelta)),
				})
			}
			doc.WriteString(
				renderTable("Output", rows) + "\n",
			)
		}
	}

	if m.err != nil {
		msg := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.err.Error())
		doc.WriteString(msg + "\n\n")
	}

	return lipgloss.NewStyle().MaxWidth(screenWidth).Render(
		doc.String(),
	) + "\n"
}

type normalizedInput struct {
	RecordDeltas []float64
	ByteDeltas   []float64
}

type normalizedOutput struct {
	ProcRecordDeltas []float64
	ProcByteDeltas   []float64
	ErrorDeltas      []float64
	RetryDeltas      []float64
	RetryFailedDelta []float64
}

func normalizeInput(history []fluentbit.Metrics, name string) normalizedInput {
	var out normalizedInput
	if len(history) < 2 {
		return out
	}

	for i, metrics := range history[1:] {
		curr, ok := metrics.Input[name]
		if !ok {
			continue
		}

		prev, ok := history[i].Input[name]
		if !ok {
			continue
		}

		out.RecordDeltas = append(out.RecordDeltas, delta(float64(curr.Records), float64(prev.Records)))
		out.ByteDeltas = append(out.ByteDeltas, delta(float64(curr.Bytes), float64(prev.Bytes)))
	}
	return out
}

func normalizeOutput(history []fluentbit.Metrics, name string) normalizedOutput {
	var out normalizedOutput
	if len(history) < 2 {
		return out
	}

	for i, metrics := range history[1:] {
		curr, ok := metrics.Output[name]
		if !ok {
			continue
		}

		prev, ok := history[i].Output[name]
		if !ok {
			continue
		}

		out.ProcRecordDeltas = append(out.ProcRecordDeltas, delta(float64(curr.ProcRecords), float64(prev.ProcRecords)))
		out.ProcByteDeltas = append(out.ProcByteDeltas, delta(float64(curr.ProcBytes), float64(prev.ProcBytes)))
		out.ErrorDeltas = append(out.ErrorDeltas, delta(float64(curr.Errors), float64(prev.Errors)))
		out.RetryDeltas = append(out.RetryDeltas, delta(float64(curr.Retries), float64(prev.Retries)))
		out.RetryFailedDelta = append(out.RetryFailedDelta, delta(float64(curr.RetriesFailed), float64(prev.RetriesFailed)))
	}
	return out
}

func currentInputNames(history []fluentbit.Metrics) []string {
	l := len(history)
	if l == 0 {
		return nil
	}

	inputs := history[l-1].Input
	out := make([]string, 0, len(inputs))
	for name := range inputs {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func currentOutputNames(history []fluentbit.Metrics) []string {
	l := len(history)
	if l == 0 {
		return nil
	}

	outputs := history[l-1].Output
	out := make([]string, 0, len(outputs))
	for name := range outputs {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

type inputDelta struct {
	RecordDeltas float64
	ByteDeltas   float64
}

func currentInputDelta(history []fluentbit.Metrics, name string) inputDelta {
	var out inputDelta
	l := len(history)
	if l < 2 {
		return out
	}

	curr, ok := history[l-1].Input[name]
	if !ok {
		return out
	}

	prev, ok := history[l-2].Input[name]
	if !ok {
		return out
	}

	out.RecordDeltas = delta(float64(curr.Records), float64(prev.Records))
	out.ByteDeltas = delta(float64(curr.Bytes), float64(prev.Bytes))

	return out
}

type outputDelta struct {
	ProcRecordDeltas float64
	ProcByteDeltas   float64
	ErrorDeltas      float64
	RetryDeltas      float64
	RetryFailedDelta float64
}

func currentOutputDelta(history []fluentbit.Metrics, name string) outputDelta {
	var out outputDelta
	l := len(history)
	if l < 2 {
		return out
	}

	curr, ok := history[l-1].Output[name]
	if !ok {
		return out
	}

	prev, ok := history[l-2].Output[name]
	if !ok {
		return out
	}

	out.ProcRecordDeltas = delta(float64(curr.ProcRecords), float64(prev.ProcRecords))
	out.ProcByteDeltas = delta(float64(curr.ProcBytes), float64(prev.ProcBytes))
	out.ErrorDeltas = delta(float64(curr.Errors), float64(prev.Errors))
	out.RetryDeltas = delta(float64(curr.Retries), float64(prev.Retries))
	out.RetryFailedDelta = delta(float64(curr.RetriesFailed), float64(prev.RetriesFailed))

	return out
}

type renderPlotProps struct {
	Caption string
	Series  []float64
	Width   int
	Height  int
}

const plotLabelGap = 5

func renderPlot(props renderPlotProps) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		PaddingRight(1).
		Width(props.Width - 2).
		Height(props.Height).
		Render(
			asciigraph.Plot(
				props.Series,
				asciigraph.Caption(props.Caption),
				asciigraph.Width((props.Width)-(labelSize(props.Series)+plotLabelGap)),
				asciigraph.Height(props.Height-2),
				asciigraph.Offset(0),
			),
		)
}

func renderTable(title string, rows [][]string) string {
	tw := table.NewWriter()
	tw.SetTitle(title)
	tw.SetStyle(table.StyleRounded)

	var thead table.Row
	for _, s := range rows[0] {
		thead = append(thead, s)
	}
	tw.AppendRow(thead)

	for _, row := range rows[1:] {
		var trow table.Row
		for _, s := range row {
			trow = append(trow, s)
		}

		tw.AppendRow(trow)
	}
	return tw.Render()
}

func delta(a, b float64) float64 {
	if a == b {
		return 0
	}

	if a > b {
		return a - b
	}

	return b - a
}

func labelSize(series []float64) int {
	ff := make([]float64, len(series))
	copy(ff, series)

	sort.Float64s(ff)
	a := len(fmt.Sprintf("%.2f", ff[0]))
	b := len(fmt.Sprintf("%.2f", ff[len(ff)-1]))
	if a > b {
		return a
	}
	return b
}
