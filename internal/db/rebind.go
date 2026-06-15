package db

import (
	"fmt"
	"strings"
)

var rebindFn = func(query string) string { return query }

func setRebind(driver string) {
	if driver == "postgres" || driver == "pgx" {
		rebindFn = rebindPostgres
	} else {
		rebindFn = func(q string) string { return q }
	}
}

func Rebind(query string) string {
	return rebindFn(query)
}

func rebindPostgres(query string) string {
	var b strings.Builder
	n := 0
	for _, c := range query {
		if c == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}