package proxyquerier

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/storage"
	"github.com/sirupsen/logrus"

	proxyconfig "github.com/promproxy/pkg/config"
	"github.com/promproxy/pkg/promclient"
	"github.com/promproxy/pkg/promutil"
)

// ProxyQuerier Implements prometheus' Querier interface
type ProxyQuerier struct {
	Ctx    context.Context
	Start  time.Time
	End    time.Time
	Client promclient.API

	Cfg *proxyconfig.PromxyConfig
}

// Select returns a set of series that matches the given label matchers.
func (h *ProxyQuerier) Select(selectParams *storage.SelectParams, matchers ...*labels.Matcher) (storage.SeriesSet, storage.Warnings, error) {
	start := time.Now()
	defer func() {
		logrus.WithFields(logrus.Fields{
			"selectParams": selectParams,
			"matchers":     matchers,
			"took":         time.Now().Sub(start),
		}).Debug("Select")
	}()

	var result model.Value
	// TODO: get warnings from lower layers
	var warnings storage.Warnings
	var err error
	// Select() is a combined API call for query/query_range/series.
	// as of right now there is no great way of differentiating between a
	// data call (query/query_range) and a metadata call (series). For now
	// the working workaround is to switch based on the selectParams.
	// https://github.com/prometheus/prometheus/issues/4057
	if selectParams == nil {
		matcherString, err := promutil.MatcherToString(matchers)
		if err != nil {
			return nil, nil, err
		}
		labelsets, w, err := h.Client.Series(h.Ctx, []string{matcherString}, h.Start, h.End)
		warnings = promutil.WarningsConvert(w)
		if err != nil {
			return nil, warnings, errors.Cause(err)
		}
		// Convert labelsets to vectors
		// convert to vector (there aren't points, but this way we don't have to make more merging functions)
		retVector := make(model.Vector, len(labelsets))
		for j, labelset := range labelsets {
			retVector[j] = &model.Sample{
				Metric: model.Metric(labelset),
			}
		}
		result = retVector
	} else {
		var w api.Warnings
		result, w, err = h.Client.GetValue(h.Ctx, timestamp.Time(selectParams.Start), timestamp.Time(selectParams.End), matchers)
		warnings = promutil.WarningsConvert(w)
	}
	if err != nil {
		return nil, warnings, errors.Cause(err)
	}

	iterators := promclient.IteratorsForValue(result)

	series := make([]storage.Series, len(iterators))
	for i, iterator := range iterators {
		series[i] = &Series{iterator}
	}

	return NewSeriesSet(series), warnings, nil
}

// LabelValues returns all potential values for a label name.
func (h *ProxyQuerier) LabelValues(name string) ([]string, storage.Warnings, error) {
	start := time.Now()
	defer func() {
		logrus.WithFields(logrus.Fields{
			"name": name,
			"took": time.Now().Sub(start),
		}).Debug("LabelValues")
	}()

	result, w, err := h.Client.LabelValues(h.Ctx, name)
	warnings := promutil.WarningsConvert(w)
	if err != nil {
		return nil, warnings, errors.Cause(err)
	}

	ret := make([]string, len(result))
	for i, r := range result {
		ret[i] = string(r)
	}

	return ret, warnings, nil
}

// LabelNames returns all the unique label names present in the block in sorted order.
func (h *ProxyQuerier) LabelNames() ([]string, storage.Warnings, error) {
	start := time.Now()
	defer func() {
		logrus.WithFields(logrus.Fields{
			"took": time.Now().Sub(start),
		}).Debug("LabelNames")
	}()

	v, w, err := h.Client.LabelNames(h.Ctx)
	return v, promutil.WarningsConvert(w), err
}

// Close closes the querier. Behavior for subsequent calls to Querier methods
// is undefined.
func (h *ProxyQuerier) Close() error { return nil }
