package main

import "testing"

func TestClassifyLogLevel(t *testing.T) {
	cases := []struct{ source, line, want string }{
		{"stdout", "[WARNING] Expected root element 'settings' but found 'mirrors'", "warn"},
		{"stderr", "[INFO] Building sa-admin", "info"},
		{"stderr", "2026-09-26  ERROR 123 --- DruidDataSource : init datasource error", "error"},
		{"stdout", "Caused by: java.net.ConnectException: Connection refused", "error"},
		{"stderr", "\tat com.mysql.cj.Driver.connect(Driver.java:42)", "detail"},
		{"stdout", "2026-09-26  WARN 123 --- BeanPostProcessorChecker : early bean", "detail"},
		{"stderr", "SLF4J(W): Class path contains multiple SLF4J providers", "detail"},
		{"stderr", "npm ERR! code EADDRINUSE", "error"},
		{"stdout", "FAILURE: Build failed with an exception.", "error"},
		{"system", "读取日志失败: pipe closed", "error"},
		{"system", "启动 sa-admin（PID 1）", "system"},
		{"stderr", "'D:\\utils\\idea\\IntelliJ' 不是内部或外部命令，也不是可运行的程序", "error"},
		{"stderr", "'mvn' is not recognized as an internal or external command", "error"},
		{"stderr", "系统找不到指定的路径。", "error"},
		{"stderr", "The JAVA_HOME environment variable is not defined correctly,", "error"},
	}
	for _, tc := range cases {
		if got := classifyLogLevel(tc.source, tc.line); got != tc.want {
			t.Errorf("classifyLogLevel(%q, %q) = %q, want %q", tc.source, tc.line, got, tc.want)
		}
	}
}
