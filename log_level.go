package main

import "strings"

// classifyLogLevel uses the message content because Maven, Java and Node often
// write ordinary information to stderr. Source alone is not a severity level.
func classifyLogLevel(source, message string) string {
	line := strings.TrimSpace(plainLogText(message))
	upper := strings.ToUpper(line)
	if source == "system" {
		if strings.Contains(line, "失败") || strings.HasPrefix(line, "Error:") {
			return "error"
		}
		return "system"
	}
	if line == "" || strings.HasPrefix(line, "at ") || strings.HasPrefix(line, "... ") || strings.HasPrefix(line, "The last packet ") {
		return "detail"
	}
	// These framework diagnostics occur during normal starts and remain in All.
	if strings.Contains(line, "BeanPostProcessorChecker") || strings.HasPrefix(line, "SLF4J(W):") {
		return "detail"
	}
	if strings.HasPrefix(upper, "[ERROR]") || strings.Contains(upper, "[ERROR]") ||
		strings.Contains(upper, " ERROR ") || strings.HasPrefix(upper, "ERROR:") ||
		strings.HasPrefix(upper, "ERROR ") || strings.HasPrefix(upper, "NPM ERR!") ||
		strings.HasPrefix(upper, "NPM ERROR") || strings.HasPrefix(upper, "FAILURE:") ||
		strings.HasPrefix(upper, "FATAL ") || strings.HasPrefix(upper, "AGGREGATEERROR") ||
		strings.Contains(upper, "HTTP PROXY ERROR:") ||
		strings.Contains(line, "不是内部或外部命令") ||
		strings.Contains(upper, "IS NOT RECOGNIZED AS AN INTERNAL OR EXTERNAL COMMAND") ||
		strings.Contains(line, "系统找不到指定的路径") || strings.Contains(line, "系统找不到指定的文件") ||
		strings.Contains(upper, "THE JAVA_HOME ENVIRONMENT VARIABLE IS NOT DEFINED CORRECTLY") ||
		strings.HasPrefix(line, "Caused by:") || strings.HasPrefix(line, "Exception in thread") ||
		strings.Contains(upper, "APPLICATION FAILED TO START") || strings.Contains(upper, "BUILD FAILURE") ||
		(strings.HasPrefix(upper, "> TASK") && strings.HasSuffix(upper, "FAILED")) {
		return "error"
	}
	if strings.HasPrefix(upper, "[WARNING]") || strings.Contains(upper, " WARN ") ||
		strings.HasPrefix(upper, "WARN ") || strings.HasPrefix(upper, "WARNING:") || strings.HasPrefix(line, "SLF4J(W)") {
		if upper == "[WARNING]" {
			return "detail"
		}
		return "warn"
	}
	if strings.HasPrefix(line, "java.") && (strings.Contains(line, "Exception:") || strings.Contains(line, "Error:")) {
		return "error"
	}
	return "info"
}
