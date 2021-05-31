package main

import (
	"reflect"
	"testing"

	fluentbit "github.com/calyptia/fluent-bit-cli"
)

func Test_normalizeInput(t *testing.T) {
	type args struct {
		history []fluentbit.Metrics
		name    string
	}
	tt := []struct {
		name string
		args args
		want normalizedInput
	}{
		{
			args: args{
				history: []fluentbit.Metrics{
					{
						Input: map[string]fluentbit.MetricInput{
							"test": {
								Records: 10,
								Bytes:   15,
							},
						},
					},
					{
						Input: map[string]fluentbit.MetricInput{
							"test": {
								Records: 8,
								Bytes:   18,
							},
						},
					},
					{
						Input: map[string]fluentbit.MetricInput{
							"test": {
								Records: 4,
								Bytes:   1,
							},
						},
					},
					{
						Input: map[string]fluentbit.MetricInput{
							"test": {
								Records: 4,
								Bytes:   1,
							},
						},
					},
					{
						Input: map[string]fluentbit.MetricInput{
							"test": {
								Records: 1,
								Bytes:   1,
							},
						},
					},
				},
				name: "test",
			},
			want: normalizedInput{
				RecordDeltas: []float64{2, 4, 0, 3},
				ByteDeltas:   []float64{3, 17, 0, 0},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeInput(tc.args.history, tc.args.name); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got: %+v\nwanted: %+v", got, tc.want)
			}
		})
	}
}
