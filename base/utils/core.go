package utils

import (
	"app/base/rbac"
	"bytes"
	"fmt"
	"math"
	"os"
	"path"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/joho/godotenv"
)

var uuidRegex = regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$")

// Getenv Load environment variable or return default value
func Getenv(key, defaultt string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultt
}

// GetenvOrFail Load environment variable or fail
func GetenvOrFail(envname string) string {
	value := os.Getenv(envname)
	if value == "" {
		panic(fmt.Sprintf("Set %s env variable!", envname))
	}

	return value
}

// FailIfEmpty Check that value is not empty otherwise fail hard
func FailIfEmpty(value string, varName string) string {
	if value == "" {
		panic(fmt.Sprintf("%s must be set!", varName))
	}

	return value
}

// parseBoolOrFail Convert string to bool or panic if not possible
func parseBoolOrFail(value string) bool {
	parsedBool, err := strconv.ParseBool(value)
	if err != nil {
		panic(err)
	}
	return parsedBool
}

// GetBoolEnvOrFail Parse bool value from env variable
func GetBoolEnvOrFail(envname string) bool {
	value := GetenvOrFail(envname)
	return parseBoolOrFail(value)
}

// GetBoolEnvOrDefault Parse bool value from env variable
func GetBoolEnvOrDefault(envname string, defval bool) bool {
	value := os.Getenv(envname)
	if value == "" {
		return defval
	}
	return parseBoolOrFail(value)
}

// parseIntOrFail Convert string to int or panic if not possible
func parseIntOrFail(envname, valueStr string) int {
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		panic(fmt.Sprintf("Unable convert '%s' env var '%s' to int!", envname, valueStr))
	}
	return value
}

// parseInt64OrFail Convert string to int or panic if not possible
func parseInt64OrFail(envname, valueStr string) int64 {
	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("Unable convert '%s' env var '%s' to int!", envname, valueStr))
	}
	return value
}

// GetIntEnvOrFail Load int environment variable or fail
func GetIntEnvOrFail(envname string) int {
	valueStr := GetenvOrFail(envname)
	return parseIntOrFail(envname, valueStr)
}

// GetIntEnvOrDefault Load int environment variable or load default
func GetIntEnvOrDefault(envname string, defval int) int {
	valueStr := os.Getenv(envname)
	if valueStr == "" {
		return defval
	}
	return parseIntOrFail(envname, valueStr)
}

// GetInt64EnvOrDefault Load int64 environment variable or load default
func GetInt64EnvOrDefault(envname string, defval int64) int64 {
	valueStr := os.Getenv(envname)
	if valueStr == "" {
		return defval
	}
	return parseInt64OrFail(envname, valueStr)
}

// SetDefaultEnvOrFail Set environment variable if not already or fail
func SetDefaultEnvOrFail(envname, value string) string {
	val := os.Getenv(envname)
	if val != "" {
		return val
	}
	err := os.Setenv(envname, value)
	if err != nil {
		panic(err)
	}

	return value
}

func TestLoadEnv(files ...string) {
	// go test changes working directory to package's location, we utilize env variable to project working directory
	dir := Getenv("TEST_WD", ".")
	for i, f := range files {
		files[i] = path.Join(dir, f)
	}
	err := godotenv.Overload(files...)

	LogDebug("files", files, "Loading new env file")
	LogDebug("dbuser", CoreCfg.DBUser, "passwd", CoreCfg.DBPassword, "Db auth info")
	if err != nil {
		LogPanic("Could not load env file")
	}
}

// LogPanics Catches panics, and logs them to stderr, then exit conditionally
func LogPanics(exitAfterLogging bool) {
	if obj := recover(); obj != nil {
		stack := string(debug.Stack())
		stackLine := strings.ReplaceAll(stack, "\n", "|")
		LogError("err", obj, "stack", stackLine, "Panicked")
		FlushLogs()
		if exitAfterLogging {
			os.Exit(1)
		}
	}
}

// SinceStr Format duration since given time as "1h2m3s
func SinceStr(tStart time.Time, precision time.Duration) string {
	return time.Since(tStart).Round(precision).String()
}

var _suffixes = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB"}

// SizeStr Format memory size to human readable
func SizeStr(size uint64) string {
	order := 0
	if size > 0 {
		order = int(math.Log2(float64(size)) / 10)
	}
	return fmt.Sprintf("%.4g%s", float64(size)/float64(int(1)<<(order*10)), _suffixes[order])
}

func IsValidUUID(uuid string) bool {
	return uuidRegex.MatchString(uuid)
}

func GetGorutineID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

func ParseInventoryGroup(id *string, name *string) (string, error) {
	group := rbac.InventoryGroup{{ID: id, Name: name}}
	groupJSON, err := sonic.Marshal(&group)
	if err != nil {
		LogError("group", group, "err", err.Error(), "Cannot Marshal Inventory group")
		return "", err
	}
	return strconv.Quote(string(groupJSON)), nil
}
