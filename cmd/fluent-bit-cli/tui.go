package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
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
	ctx               context.Context
	baseURLSet        bool
	baseURLInput      textinput.Model
	baseURL           string
	pullIntervalSet   bool
	pullIntervalInput textinput.Model
	pullInterval      time.Duration
	fluentbit         *fluentbit.Client
	indexes           int
	selectedIndex     int
	series            *fluentbit.Series
	infoLoaded        bool
	info              fluentbit.BuildInfo
	err               error
}

func initialModel(ctx context.Context) model {
	baseURLInput := textinput.NewModel()
	baseURLInput.Placeholder = "Fluent Bit Base URL"
	baseURLInput.Width = 40
	baseURLInput.SetValue("http://localhost:2020")
	baseURLInput.Focus()

	pullIntervalInput := textinput.NewModel()
	pullIntervalInput.Placeholder = "Pull interval (seconds)"
	pullIntervalInput.Width = 19
	pullIntervalInput.SetValue("5")
	return model{
		ctx:               ctx,
		baseURLInput:      baseURLInput,
		pullIntervalInput: pullIntervalInput,
		series: &fluentbit.Series{
			Input:  map[string]fluentbit.InputSeries{},
			Output: map[string]fluentbit.OutputSeries{},
		},
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

func fetchMetricsCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
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
				if !m.baseURLSet {
					baseURL := m.baseURLInput.Value()
					if u, err := url.Parse(baseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
						m.err = errors.New("invalid URL")
						return m, textinput.Blink
					}

					m.baseURL = baseURL
					m.baseURLSet = true
					m.pullIntervalInput.Focus()
				} else {
					pullIntervalStr := m.pullIntervalInput.Value()
					pullIntervalInt, err := strconv.ParseInt(pullIntervalStr, 10, 64)
					if err != nil || pullIntervalInt < 1 {
						m.err = errors.New("invalid pull interval")
						return m, textinput.Blink
					}

					m.fluentbit = &fluentbit.Client{
						HTTPClient: http.DefaultClient,
						BaseURL:    strings.TrimSuffix(m.baseURL, "/"),
					}

					m.pullInterval = time.Second * time.Duration(pullIntervalInt)
					m.pullIntervalSet = true

					return m, tea.Batch(fetchBuildInfoCmd(), fetchMetricsCmd(m.pullInterval))
				}
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
			ctx, cancel := context.WithTimeout(m.ctx, time.Second+1)
			defer cancel()

			err := m.series.Push(func() (fluentbit.Metrics, error) {
				mm, err := m.fluentbit.Metrics(ctx)
				if err != nil {
					return mm, err
				}
				m.indexes = len(mm.Input) + len(mm.Output)
				return mm, nil
			})
			if err != nil {
				m.err = err
				return m, fetchMetricsCmd(m.pullInterval)
			}

			m.err = nil
			return m, fetchMetricsCmd(m.pullInterval)
		}()
	}

	if m.fluentbit == nil {
		var cmd1, cmd2 tea.Cmd
		m.baseURLInput, cmd1 = m.baseURLInput.Update(msg)
		m.pullIntervalInput, cmd2 = m.pullIntervalInput.Update(msg)
		return m, tea.Batch(cmd1, cmd2)
	}

	return m, nil
}

func (m model) View() string {
	var doc strings.Builder

	if m.fluentbit == nil {
		if !m.baseURLSet {
			doc.WriteString(
				"Fluent Bit Base URL\n" + m.baseURLInput.View() + "\n",
			)
		} else {
			doc.WriteString(
				"Pull interval (seconds)\n" +
					m.pullIntervalInput.View() + "\n",
			)
		}
	}

	var selectedType, selectedMetric string
	inputNames := m.series.InputNames()
	outputNames := m.series.OutputNames()
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

	switch selectedType {
	case "input":
		rates := m.series.Input[selectedMetric].InstantRates()
		doc.WriteString(
			lipgloss.JoinHorizontal(lipgloss.Bottom,
				renderPlot(renderPlotProps{
					Caption: "input " + selectedMetric + " records rate",
					Series:  uint64ToFloat64Slice(rates.Records),
					Width:   screenWidth / 2,
					Height:  12,
				}),
				renderPlot(renderPlotProps{
					Caption: "input " + selectedMetric + " bytes rate",
					Series:  uint64ToFloat64Slice(rates.Bytes),
					Width:   screenWidth / 2,
					Height:  12,
				}),
			) + "\n",
		)
	case "output":
		rates := m.series.Output[selectedMetric].InstantRates()
		doc.WriteString(
			lipgloss.JoinVertical(lipgloss.Left,
				lipgloss.JoinHorizontal(lipgloss.Bottom,
					renderPlot(renderPlotProps{
						Caption: "output " + selectedMetric + " proc_records rate",
						Series:  uint64ToFloat64Slice(rates.ProcRecords),
						Width:   screenWidth / 2,
						Height:  12,
					}),
					renderPlot(renderPlotProps{
						Caption: "output " + selectedMetric + " proc_bytes rate",
						Series:  uint64ToFloat64Slice(rates.ProcBytes),
						Width:   screenWidth / 2,
						Height:  12,
					}),
				),
				lipgloss.JoinHorizontal(lipgloss.Bottom,
					renderPlot(renderPlotProps{
						Caption: "output " + selectedMetric + " errors rate",
						Series:  uint64ToFloat64Slice(rates.Errors),
						Width:   screenWidth / 3,
						Height:  7,
					}),
					renderPlot(renderPlotProps{
						Caption: "output " + selectedMetric + " retries rate",
						Series:  uint64ToFloat64Slice(rates.Retries),
						Width:   screenWidth / 3,
						Height:  7,
					}),
					renderPlot(renderPlotProps{
						Caption: "output " + selectedMetric + " retries_failed rate",
						Series:  uint64ToFloat64Slice(rates.RetriesFailed),
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

			series := m.series.Input[name]
			i := len(series.Records) // all fields are equal length
			if i != 0 {
				i--
			}
			rows = append(rows, []string{
				style.Render(name),
				style.Render(fmt.Sprintf("%d", series.Records[i])),
				style.Render(fmt.Sprintf("%d", series.Bytes[i])),
			})
		}
		doc.WriteString(
			renderTable("Inputs", rows) + "\n",
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

			series := m.series.Output[name]
			i := len(series.ProcRecords) // all fields are equal length
			if i != 0 {
				i--
			}
			rows = append(rows, []string{
				style.Render(name),
				style.Render(fmt.Sprintf("%d", series.ProcRecords[i])),
				style.Render(fmt.Sprintf("%d", series.ProcBytes[i])),
				style.Render(fmt.Sprintf("%d", series.Errors[i])),
				style.Render(fmt.Sprintf("%d", series.Retries[i])),
				style.Render(fmt.Sprintf("%d", series.RetriesFailed[i])),
			})
		}
		doc.WriteString(
			renderTable("Outputs", rows) + "\n",
		)
	}

	if m.err != nil {
		msg := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.err.Error())
		doc.WriteString(msg + "\n\n")
	}

	return lipgloss.NewStyle().MaxWidth(screenWidth).Render(
		doc.String(),
	) + "\n"
}

type renderPlotProps struct {
	Caption string
	Series  []float64
	Width   int
	Height  int
}

const plotLabelGap = 5

func renderPlot(props renderPlotProps) string {
	var content string
	if len(props.Series) == 0 {
		content = "Loading..."
	} else {
		content = asciigraph.Plot(
			props.Series,
			asciigraph.Caption(props.Caption),
			asciigraph.Width((props.Width)-(labelSize(props.Series)+plotLabelGap)),
			asciigraph.Height(props.Height-2),
			asciigraph.Offset(0),
		)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		PaddingRight(1).
		Width(props.Width - 2).
		Height(props.Height).
		Render(content)
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

func uint64ToFloat64Slice(vv []uint64) []float64 {
	l := len(vv)
	if l == 0 {
		return nil
	}

	out := make([]float64, l)
	for i, v := range vv {
		out[i] = float64(v)
	}
	return out
}
