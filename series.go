package fluentbit

import (
	"sort"
	"sync"
)

type Series struct {
	Input  map[string]InputSeries
	Output map[string]OutputSeries
	mu     sync.Mutex
}

func (ss *Series) Push(metrics func() (Metrics, error)) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	mm, err := metrics()
	if err != nil {
		return err
	}

	for name, input := range mm.Input {
		series, ok := ss.Input[name]
		if !ok {
			ss.Input[name] = InputSeries{
				Records: []uint64{input.Records},
				Bytes:   []uint64{input.Bytes},
			}
			continue
		}

		series.Records = append(series.Records, input.Records)
		series.Bytes = append(series.Bytes, input.Bytes)
		ss.Input[name] = series
	}

	for name, output := range mm.Output {
		series, ok := ss.Output[name]
		if !ok {
			ss.Output[name] = OutputSeries{
				ProcRecords:   []uint64{output.ProcRecords},
				ProcBytes:     []uint64{output.ProcBytes},
				Errors:        []uint64{output.Errors},
				Retries:       []uint64{output.Retries},
				RetriesFailed: []uint64{output.RetriesFailed},
			}
			continue
		}

		series.ProcRecords = append(series.ProcRecords, output.ProcRecords)
		series.ProcBytes = append(series.ProcBytes, output.ProcBytes)
		series.Errors = append(series.Errors, output.Errors)
		series.Retries = append(series.Retries, output.Retries)
		series.RetriesFailed = append(series.RetriesFailed, output.RetriesFailed)
		ss.Output[name] = series
	}

	return nil
}

func (ss *Series) InputNames() []string {
	l := len(ss.Input)
	if l == 0 {
		return nil
	}

	out := make([]string, 0, l)
	for name := range ss.Input {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (ss *Series) OutputNames() []string {
	l := len(ss.Output)
	if l == 0 {
		return nil
	}

	out := make([]string, 0, l)
	for name := range ss.Output {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

type InputSeries struct {
	Records []uint64
	Bytes   []uint64
}

// InstantRates converts the series counters into rates.
func (ss InputSeries) InstantRates() InputSeries {
	var out InputSeries
	if l := len(ss.Records); l > 2 {
		for i := 2; i < l+1; i++ {
			prev := ss.Records[i-2]
			curr := ss.Records[i-1]
			if reset := curr < prev; !reset {
				out.Records = append(out.Records, curr-prev)
			}

			prev = ss.Bytes[i-2]
			curr = ss.Bytes[i-1]
			if reset := curr < prev; !reset {
				out.Bytes = append(out.Bytes, curr-prev)
			}
		}
	}
	return out
}

type OutputSeries struct {
	ProcRecords   []uint64
	ProcBytes     []uint64
	Errors        []uint64
	Retries       []uint64
	RetriesFailed []uint64
}

// InstantRates converts the series counters into rates.
func (ss OutputSeries) InstantRates() OutputSeries {
	var out OutputSeries
	if l := len(ss.ProcRecords); l > 2 {
		for i := 2; i < l+1; i++ {
			prev := ss.ProcRecords[i-2]
			curr := ss.ProcRecords[i-1]
			if reset := curr < prev; !reset {
				out.ProcRecords = append(out.ProcRecords, curr-prev)
			}

			prev = ss.ProcBytes[i-2]
			curr = ss.ProcBytes[i-1]
			if reset := curr < prev; !reset {
				out.ProcBytes = append(out.ProcBytes, curr-prev)
			}

			prev = ss.Errors[i-2]
			curr = ss.Errors[i-1]
			if reset := curr < prev; !reset {
				out.Errors = append(out.Errors, curr-prev)
			}

			prev = ss.Retries[i-2]
			curr = ss.Retries[i-1]
			if reset := curr < prev; !reset {
				out.Retries = append(out.Retries, curr-prev)
			}

			prev = ss.RetriesFailed[i-2]
			curr = ss.RetriesFailed[i-1]
			if reset := curr < prev; !reset {
				out.RetriesFailed = append(out.RetriesFailed, curr-prev)
			}
		}
	}
	return out
}
