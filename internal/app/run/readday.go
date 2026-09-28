package run

// readDays reads a calendar's days — a date and a value each, in date order —
// and shades each by the quantile of the days it falls in, as a heatmap's
// cells are, so a quiet month and a busy one both use the whole ramp.
func readDays(rows Rows) ([]Day, error) {
	out := []Day{}
	var values []float64
	for rows.Next() {
		var at, v any
		if err := rows.Scan(&at, &v); err != nil {
			return nil, err
		}
		t, ok := asTime(at)
		if !ok {
			// A day with no date is not a place on a calendar.
			continue
		}
		n, _ := number(v)
		out = append(out, Day{Date: t.Format("2006-01-02"), Value: n, Formatted: compact(n)})
		values = append(values, n)
	}
	breaks := steps(values)
	for i := range out {
		out[i].Step = stepOf(out[i].Value, breaks)
	}
	return out, rows.Err()
}
