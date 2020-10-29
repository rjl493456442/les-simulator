package simulator

import (
	"strconv"
	"strings"
)

func ParseTopology(connstr string, maxFrom, maxTo int) []*Conn {
	instructions := strings.Split(connstr, ",")
	if len(instructions) == 0 {
		return nil
	}
	var conns []*Conn
	for _, instr := range instructions {
		trimmed := strings.TrimSpace(instr)
		if len(trimmed) == 0 {
			continue
		}
		elems := strings.Split(trimmed, "->")
		if len(elems) != 2 {
			continue
		}
		var (
			from               = elems[0]
			to                 = elems[1]
			fromIndex, toIndex int
		)
		switch {
		case from == "*":
			fromIndex = -1
		case strings.HasPrefix(from, "c"):
			fallthrough
		case strings.HasPrefix(from, "C"):
			parsed, err := strconv.Atoi(from[1:])
			if err != nil {
				continue
			}
			if parsed < 0 || parsed >= maxFrom {
				continue
			}
			fromIndex = parsed
		default:
			continue
		}

		switch {
		case to == "*":
			toIndex = -1
		case strings.HasPrefix(to, "s"):
			fallthrough
		case strings.HasPrefix(to, "S"):
			parsed, err := strconv.Atoi(to[1:])
			if err != nil {
				continue
			}
			if parsed < 0 || parsed >= maxTo {
				continue
			}
			toIndex = parsed
		default:
			continue
		}
		// Append the real connection
		if fromIndex != -1 && toIndex != -1 {
			conns = append(conns, &Conn{
				From: fromIndex,
				To:   toIndex,
			})
			continue
		}
		if fromIndex == -1 && toIndex == -1 {
			for i := 0; i < maxFrom; i++ {
				for j := 0; j < maxTo; j++ {
					conns = append(conns, &Conn{
						From: i,
						To:   j,
					})
				}
			}
			continue
		}
		if fromIndex == -1 {
			for i := 0; i < maxFrom; i++ {
				conns = append(conns, &Conn{
					From: i,
					To:   toIndex,
				})
			}
			continue
		}
		for i := 0; i < maxTo; i++ {
			conns = append(conns, &Conn{
				From: fromIndex,
				To:   i,
			})
		}
	}
	return conns
}
