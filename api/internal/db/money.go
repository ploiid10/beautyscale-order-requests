package db

import (
	"fmt"
	"math"
	"strings"
)

func applyTotals(lines []Line) (string, int64, error) {
	var currency string
	var total int64
	for i := range lines {
		lineTotal, err := lineTotal(lines[i].UnitPriceMinor, lines[i].Quantity)
		if err != nil {
			return "", 0, err
		}
		lines[i].LineTotalMinor = lineTotal
		lines[i].Currency = strings.TrimSpace(lines[i].Currency)
		if lines[i].Currency != "" {
			if currency == "" {
				currency = lines[i].Currency
			} else if currency != lines[i].Currency {
				return "", 0, fmt.Errorf("mixed currency on order request")
			}
		}
		if total > math.MaxInt64-lineTotal {
			return "", 0, fmt.Errorf("total overflow")
		}
		total += lineTotal
	}
	return currency, total, nil
}

func lineTotal(price int64, qty int) (int64, error) {
	if price < 0 || qty < 0 {
		return 0, fmt.Errorf("negative money or quantity")
	}
	q := int64(qty)
	if price > 0 && q > math.MaxInt64/price {
		return 0, fmt.Errorf("line total overflow")
	}
	return price * q, nil
}
