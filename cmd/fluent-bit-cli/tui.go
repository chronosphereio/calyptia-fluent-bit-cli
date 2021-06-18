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

var (
	errInvalidBaseURL      = errors.New("invalid base URL")
	errInvalidPullInterval = errors.New("invalid pull interval")
)

type model struct {
	ctx context.Context

	page string

	baseURL      *url.URL
	baseURLInput textinput.Model

	pullInterval      time.Duration
	pullIntervalInput textinput.Model

	fluentbit *fluentbit.Client

	indexes       int
	selectedIndex int

	foundSelection bool
	selectedType   string
	selectedMetric string

	series *fluentbit.Series

	infoLoaded bool
	info       fluentbit.BuildInfo

	err error
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
		page:              "settings",
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

func updateSelectedMetric(m model) model {
	inputNames := m.series.InputNames()
	outputNames := m.series.OutputNames()
	lenInputs := len(inputNames)
	lenOutputs := len(outputNames)
	if lenInputs != 0 && m.selectedIndex >= 0 && m.selectedIndex < lenInputs {
		m.selectedType = "input"
		m.selectedMetric = inputNames[m.selectedIndex]
		m.foundSelection = true
	} else if lenOutputs != 0 && m.selectedIndex >= 0 && m.selectedIndex < (lenInputs+lenOutputs) {
		m.selectedType = "output"
		m.selectedMetric = outputNames[m.selectedIndex-lenInputs]
		m.foundSelection = true
	} else {
		m.foundSelection = false
	}
	return m
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			if m.page == "graph" {
				m.page = "table"
			}
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			if m.page == "settings" {
				if m.baseURL == nil {
					u, err := url.Parse(m.baseURLInput.Value())
					if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
						m.err = errInvalidBaseURL
						return m, textinput.Blink
					}

					u.RawQuery = ""
					u.Fragment = ""
					u.Path = ""
					if m.err == errInvalidBaseURL {
						m.err = nil
					}
					m.baseURL = u
					m.pullIntervalInput.Focus()
				} else {
					pullIntervalInt, err := strconv.ParseInt(m.pullIntervalInput.Value(), 10, 64)
					if err != nil || pullIntervalInt < 1 {
						m.err = errInvalidPullInterval
						return m, textinput.Blink
					}

					m.fluentbit = &fluentbit.Client{
						HTTPClient: http.DefaultClient,
						BaseURL:    m.baseURL.String(),
					}

					if m.err == errInvalidPullInterval {
						m.err = nil
					}
					m.pullInterval = time.Second * time.Duration(pullIntervalInt)
					m.page = "table"

					return m, tea.Batch(fetchBuildInfoCmd(), fetchMetricsCmd(m.pullInterval))
				}
			}
			if m.page == "table" && m.foundSelection {
				m.page = "graph"
			}
		case tea.KeyShiftTab, tea.KeyLeft, tea.KeyUp:
			if m.page == "table" {
				m.selectedIndex--
				if m.selectedIndex == -1 {
					m.selectedIndex = 0
				}
				m = updateSelectedMetric(m)
				return m, nil
			}
		case tea.KeyTab, tea.KeyRight, tea.KeyDown:
			if m.page == "table" {
				m.selectedIndex++
				if m.selectedIndex == m.indexes {
					m.selectedIndex = m.indexes - 1
					if m.selectedIndex == -1 {
						m.selectedIndex = 0
					}
				}
				m = updateSelectedMetric(m)
				return m, nil
			}
		case tea.KeySpace:
			if m.page == "table" && m.foundSelection {
				m.page = "graph"
			}
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
			m = updateSelectedMetric(m)
			if err != nil {
				m.err = err
				return m, fetchMetricsCmd(m.pullInterval)
			}

			m.err = nil
			return m, fetchMetricsCmd(m.pullInterval)
		}()
	}

	if m.page == "settings" {
		var cmd1, cmd2 tea.Cmd
		m.baseURLInput, cmd1 = m.baseURLInput.Update(msg)
		m.pullIntervalInput, cmd2 = m.pullIntervalInput.Update(msg)
		return m, tea.Batch(cmd1, cmd2)
	}

	return m, nil
}

func (m model) View() string {
	var doc strings.Builder

	if m.page == "settings" {
		if m.baseURL == nil {
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

	screenWidth, screenHeight, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		screenWidth = 80
		screenHeight = 32
	}

	var statusLines []string
	if m.infoLoaded {
		statusLines = append(statusLines, fmt.Sprintf("fluent-bit version: %s (%s)", m.info.FluentBit.Version, m.info.FluentBit.Edition))
	}

	if m.baseURL != nil {
		statusLines = append(statusLines, fmt.Sprintf("host: %s", m.baseURL.Host))
	}

	if m.pullInterval != 0 {
		statusLines = append(statusLines, fmt.Sprintf("pull interval: %ds", int64(m.pullInterval.Seconds())))
	}

	if m.err != nil {
		statusLines = append(statusLines, " "+lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(fmt.Sprintf("error: %s", m.err.Error())))
	}

	var statusBar string
	var statusBarHeight int
	if len(statusLines) != 0 {
		for i, s := range statusLines {
			if i != len(statusLines)-1 {
				statusLines[i] = s + " "
			}
		}

		statusBar = lipgloss.NewStyle().
			Width(screenWidth).
			Background(lipgloss.Color("6")).
			Foreground(lipgloss.Color("0")).
			Padding(0, 1).
			Render(
				lipgloss.JoinHorizontal(lipgloss.Left, statusLines...),
			)
		statusBarHeight = lipgloss.Height(statusBar)
	}

	if m.page == "table" {
		inputNames := m.series.InputNames()
		outputNames := m.series.OutputNames()
		lenInputs := len(inputNames)
		lenOutputs := len(outputNames)

		var tables []string
		if lenInputs != 0 {
			rows := [][]string{{
				"name",
				"records",
				"bytes",
			}}
			for _, name := range inputNames {
				style := lipgloss.NewStyle()
				if m.selectedType == "input" && m.selectedMetric == name {
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
			tables = append(tables, renderTable("Inputs", rows))
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
				if m.selectedType == "output" && m.selectedMetric == name {
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
			tables = append(tables, renderTable("Outputs", rows))
		}

		if len(tables) != 0 {
			doc.WriteString(
				lipgloss.JoinVertical(lipgloss.Left,
					lipgloss.JoinVertical(lipgloss.Left, tables...),
					lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("(Move with arrow keys, and select with <Enter>)"),
				) + "\n",
			)
		}
	}

	if m.page == "graph" {
		switch m.selectedType {
		case "input":
			rates := m.series.Input[m.selectedMetric].InstantRates()
			doc.WriteString(
				lipgloss.JoinVertical(lipgloss.Left,
					lipgloss.NewStyle().Background(lipgloss.Color("6")).Foreground(lipgloss.Color("0")).Padding(0, 1).Render("Input "+m.selectedMetric),
					lipgloss.JoinHorizontal(lipgloss.Bottom,
						renderPlot(renderPlotProps{
							Caption: "records rate",
							Series:  uint64ToFloat64Slice(rates.Records),
							Width:   screenWidth / 2,
							Height:  screenHeight - (7 + statusBarHeight),
						}),
						renderPlot(renderPlotProps{
							Caption: "bytes rate",
							Series:  uint64ToFloat64Slice(rates.Bytes),
							Width:   screenWidth / 2,
							Height:  screenHeight - (7 + statusBarHeight),
						}),
					),
					lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("(Press <Esc> to go back)"),
				) + "\n",
			)
		case "output":
			rates := m.series.Output[m.selectedMetric].InstantRates()
			doc.WriteString(
				lipgloss.JoinVertical(lipgloss.Left,
					lipgloss.NewStyle().Background(lipgloss.Color("6")).Foreground(lipgloss.Color("0")).Padding(0, 1).Render("Output "+m.selectedMetric),
					lipgloss.JoinHorizontal(lipgloss.Bottom,
						renderPlot(renderPlotProps{
							Caption: "proc_records rate",
							Series:  uint64ToFloat64Slice(rates.ProcRecords),
							Width:   screenWidth / 2,
							Height:  (screenHeight / 2) - (2 + statusBarHeight),
						}),
						renderPlot(renderPlotProps{
							Caption: "proc_bytes rate",
							Series:  uint64ToFloat64Slice(rates.ProcBytes),
							Width:   screenWidth / 2,
							Height:  (screenHeight / 2) - (2 + statusBarHeight),
						}),
					),
					lipgloss.JoinHorizontal(lipgloss.Bottom,
						renderPlot(renderPlotProps{
							Caption: "errors rate",
							Series:  uint64ToFloat64Slice(rates.Errors),
							Width:   screenWidth / 3,
							Height:  (screenHeight / 2) - 7,
						}),
						renderPlot(renderPlotProps{
							Caption: "retries rate",
							Series:  uint64ToFloat64Slice(rates.Retries),
							Width:   screenWidth / 3,
							Height:  (screenHeight / 2) - 7,
						}),
						renderPlot(renderPlotProps{
							Caption: "retries_failed rate",
							Series:  uint64ToFloat64Slice(rates.RetriesFailed),
							Width:   screenWidth / 3,
							Height:  (screenHeight / 2) - 7,
						}),
					),
					lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("(Press <Esc> to go back)"),
				) + "\n",
			)
		}
	}

	if m.err != nil {
		msg := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.err.Error())
		doc.WriteString(msg + "\n\n")
	}

	if statusBar != "" {
		docHeight := lipgloss.Height(doc.String())

		diff := (screenHeight - docHeight) - 2
		if diff > 0 {
			doc.WriteString(strings.Repeat("\n", diff))
		}

		doc.WriteString(statusBar)
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
		content = " Loading..."
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
