package fluentbit

import (
	"errors"
	"reflect"
	"testing"
)

func newSeries() *Series {
	return &Series{
		Input:  map[string]InputSeries{},
		Output: map[string]OutputSeries{},
	}
}

func TestSeries_Push(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		ss := newSeries()
		err := ss.Push(func() (Metrics, error) {
			return Metrics{}, errors.New("some error")
		})
		if want, got := errors.New("some error"), err; want.Error() != got.Error() {
			t.Errorf("want error %v; got %v", want, got)
		}
	})
	t.Run("ok", func(t *testing.T) {
		ss := newSeries()
		err := ss.Push(func() (Metrics, error) {
			return Metrics{
				Input: map[string]MetricInput{
					"test": {
						Records: 1,
						Bytes:   10,
					},
				},
				Output: map[string]MetricOutput{
					"test": {
						ProcRecords:   1,
						ProcBytes:     10,
						Errors:        0,
						Retries:       0,
						RetriesFailed: 0,
					},
				},
			}, nil
		})
		if err != nil {
			t.Errorf("want error nil; got %v", err)
			return
		}

		err = ss.Push(func() (Metrics, error) {
			return Metrics{
				Input: map[string]MetricInput{
					"test": {
						Records: 5,
						Bytes:   15,
					},
					"test2": {
						Records: 1,
						Bytes:   10,
					},
				},
				Output: map[string]MetricOutput{
					"test": {
						ProcRecords:   8,
						ProcBytes:     60,
						Errors:        1,
						Retries:       5,
						RetriesFailed: 1,
					},
				},
			}, nil
		})
		if err != nil {
			t.Errorf("want error nil; got %v", err)
			return
		}

		wantInputSeries := InputSeries{
			Records: []uint64{1, 5},
			Bytes:   []uint64{10, 15},
		}
		if got := ss.Input["test"]; !reflect.DeepEqual(wantInputSeries, got) {
			t.Errorf("want input series %+v; got %+v", wantInputSeries, got)
		}

		wantInputSeries2 := InputSeries{
			Records: []uint64{1},
			Bytes:   []uint64{10},
		}
		if got := ss.Input["test2"]; !reflect.DeepEqual(wantInputSeries2, got) {
			t.Errorf("want input series %+v; got %+v", wantInputSeries2, got)
		}

		wantOutputSeries := OutputSeries{
			ProcRecords:   []uint64{1, 8},
			ProcBytes:     []uint64{10, 60},
			Errors:        []uint64{0, 1},
			Retries:       []uint64{0, 5},
			RetriesFailed: []uint64{0, 1},
		}
		if got := ss.Output["test"]; !reflect.DeepEqual(wantOutputSeries, got) {
			t.Errorf("want output series %+v; got %+v", wantOutputSeries, got)
		}
	})
}

func TestSeries_InputNames(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ss := newSeries()
		got := ss.InputNames()
		if got != nil {
			t.Errorf("want input names nil; got %+v", got)
		}
	})
	t.Run("ok", func(t *testing.T) {
		ss := newSeries()
		ss.Input["test1"] = InputSeries{}
		ss.Input["test2"] = InputSeries{}
		ss.Input["test3"] = InputSeries{}
		got := ss.InputNames()
		want := []string{"test1", "test2", "test3"}

		if !reflect.DeepEqual(want, got) {
			t.Errorf("want input names %+v; got %+v", want, got)
		}
	})
}

func TestSeries_OutputNames(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ss := newSeries()
		got := ss.OutputNames()
		if got != nil {
			t.Errorf("want output names nil; got %+v", got)
		}
	})
	t.Run("ok", func(t *testing.T) {
		ss := newSeries()
		ss.Output["test1"] = OutputSeries{}
		ss.Output["test2"] = OutputSeries{}
		ss.Output["test3"] = OutputSeries{}
		got := ss.OutputNames()
		want := []string{"test1", "test2", "test3"}

		if !reflect.DeepEqual(want, got) {
			t.Errorf("want output names %+v; got %+v", want, got)
		}
	})
}

func TestInputSeries_InstantRates(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		series := InputSeries{}
		got := series.InstantRates()
		if got.Records != nil {
			t.Errorf("want input instant rates (records) nil; got %+v", got.Records)
		}
		if got.Bytes != nil {
			t.Errorf("want input instant rates (bytes) nil; got %+v", got.Bytes)
		}
	})

	t.Run("ok", func(t *testing.T) {
		series := InputSeries{
			Records: []uint64{1, 3, 6, 40},
			Bytes:   []uint64{40, 0, 9, 48},
		}
		got := series.InstantRates()
		wantRecords := []uint64{2, 3, 34}
		if !reflect.DeepEqual(wantRecords, got.Records) {
			t.Errorf("want input instant rates (records) %+v; got %+v", wantRecords, got.Records)
		}
		wantBytes := []uint64{9, 39}
		if !reflect.DeepEqual(wantBytes, got.Bytes) {
			t.Errorf("want input instant rates (bytes) %+v; got %+v", wantBytes, got.Bytes)
		}
	})
}

func TestOutputSeries_InstantRates(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		series := OutputSeries{}
		got := series.InstantRates()
		if got.ProcRecords != nil {
			t.Errorf("want output instant rates (records) nil; got %+v", got.ProcRecords)
		}
		if got.ProcBytes != nil {
			t.Errorf("want output instant rates (bytes) nil; got %+v", got.ProcBytes)
		}
	})

	t.Run("ok", func(t *testing.T) {
		series := OutputSeries{
			ProcRecords:   []uint64{4, 7, 10, 50},
			ProcBytes:     []uint64{60, 0, 6, 10},
			Errors:        []uint64{4, 7, 0, 5},
			Retries:       []uint64{4, 1, 45, 3},
			RetriesFailed: []uint64{90, 8, 1, 1},
		}
		got := series.InstantRates()
		wantProcRecords := []uint64{3, 3, 40}
		if !reflect.DeepEqual(wantProcRecords, got.ProcRecords) {
			t.Errorf("want output instant rates (proc_records) %+v; got %+v", wantProcRecords, got.ProcRecords)
		}
		wantProcBytes := []uint64{6, 4}
		if !reflect.DeepEqual(wantProcBytes, got.ProcBytes) {
			t.Errorf("want output instant rates (proc_bytes) %+v; got %+v", wantProcBytes, got.ProcBytes)
		}
		wantErrors := []uint64{3, 5}
		if !reflect.DeepEqual(wantErrors, got.Errors) {
			t.Errorf("want output instant rates (errors) %+v; got %+v", wantErrors, got.Errors)
		}
		wantRetries := []uint64{44}
		if !reflect.DeepEqual(wantRetries, got.Retries) {
			t.Errorf("want output instant rates (retries) %+v; got %+v", wantRetries, got.Retries)
		}
		wantRetriesFailed := []uint64{0}
		if !reflect.DeepEqual(wantRetriesFailed, got.RetriesFailed) {
			t.Errorf("want output instant rates (retries) %+v; got %+v", wantRetriesFailed, got.RetriesFailed)
		}
	})
}
