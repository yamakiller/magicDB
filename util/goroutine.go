package util

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

//GetCurrentGoroutineID desc
//@method GetCurrentGoroutineID desc: Return the ID of the current coroutine
//@return  (int)
func GetCurrentGoroutineID() int {
	defer func() {
		if err := recover(); err != nil {
			panic(err)
		}
	}()

	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	idField := strings.Fields(strings.TrimPrefix(string(buf[:n]), "goroutine "))[0]
	id, err := strconv.Atoi(idField)
	Assert(err == nil, fmt.Sprintf("cannot get goroutine id: %v", err))
	return id
}
