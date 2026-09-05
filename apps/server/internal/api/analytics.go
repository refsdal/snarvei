package api

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
)

const defaultAnalyticsDays = 30

func (d Deps) GetLinkAnalytics(ctx context.Context, req gen.GetLinkAnalyticsRequestObject) (gen.GetLinkAnalyticsResponseObject, error) {
	row, _, err := d.linkForCaller(ctx, req.LinkId)
	if err != nil {
		return nil, err
	}
	days := defaultAnalyticsDays
	if req.Params.Days != nil {
		days = *req.Params.Days
	}
	if days < 1 || days > 365 {
		return nil, fail(400, "VALIDATION_FAILED", "days must be 1..365")
	}
	to := time.Now().UTC()
	from := to.Add(-time.Duration(days) * 24 * time.Hour)
	tz := func(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

	totals, err := d.Q.AnalyticsTotals(ctx, dbgen.AnalyticsTotalsParams{LinkID: row.ID, ClickedAt: tz(from), ClickedAt_2: tz(to)})
	if err != nil {
		return nil, err
	}
	byDay, err := d.Q.AnalyticsByDay(ctx, dbgen.AnalyticsByDayParams{LinkID: row.ID, ClickedAt: tz(from), ClickedAt_2: tz(to)})
	if err != nil {
		return nil, err
	}
	refs, err := d.Q.AnalyticsTopReferers(ctx, dbgen.AnalyticsTopReferersParams{LinkID: row.ID, ClickedAt: tz(from), ClickedAt_2: tz(to)})
	if err != nil {
		return nil, err
	}
	countries, err := d.Q.AnalyticsTopCountries(ctx, dbgen.AnalyticsTopCountriesParams{LinkID: row.ID, ClickedAt: tz(from), ClickedAt_2: tz(to)})
	if err != nil {
		return nil, err
	}

	out := gen.Analytics{TotalClicks: int(totals.TotalClicks), UniqueVisitorApproximation: int(totals.UniqueVisitors)}
	out.ClicksByDay = make([]struct {
		Clicks int    `json:"clicks"`
		Day    string `json:"day"`
	}, 0, len(byDay))
	for _, r := range byDay {
		out.ClicksByDay = append(out.ClicksByDay, struct {
			Clicks int    `json:"clicks"`
			Day    string `json:"day"`
		}{Clicks: int(r.Clicks), Day: r.Day})
	}
	out.TopReferrers = make([]struct {
		Clicks  int     `json:"clicks"`
		Referer *string `json:"referer"`
	}, 0, len(refs))
	for _, r := range refs {
		out.TopReferrers = append(out.TopReferrers, struct {
			Clicks  int     `json:"clicks"`
			Referer *string `json:"referer"`
		}{Clicks: int(r.Clicks), Referer: r.Referer})
	}
	out.TopCountries = make([]struct {
		Clicks  int     `json:"clicks"`
		Country *string `json:"country"`
	}, 0, len(countries))
	for _, r := range countries {
		out.TopCountries = append(out.TopCountries, struct {
			Clicks  int     `json:"clicks"`
			Country *string `json:"country"`
		}{Clicks: int(r.Clicks), Country: r.Country})
	}
	out.Range.From, out.Range.To = from, to
	return gen.GetLinkAnalytics200JSONResponse(out), nil
}
