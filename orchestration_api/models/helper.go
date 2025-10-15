package models

import (
	"fmt"
	"strconv"
	"time"
)

func GetTimeFromUnixTimestamp(unixTimestampStr string) time.Time {
	timestampInt, err := strconv.ParseInt(unixTimestampStr, 10, 64)
	if err != nil {
		fmt.Println("Error parsing timestamp:", err)
		panic(err)
	}

	return time.Unix(timestampInt, 0)
}