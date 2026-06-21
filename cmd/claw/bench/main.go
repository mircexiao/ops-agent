package main

import (
	"context"

	"github.com/mircexiao/go-tiny-claw/internal/eval"
)

func main() {

	testCases := []eval.TestCase{
		{
			ID:   "test_001_edit",
			Name: "测试模糊替换工具的准确性",
			// 准备靶机：生成一个有错误的 json 文件
			SetupScript: `'{"name": "tiny-claw", "version": "v1.0.0"}' | Out-File -FilePath config.json -Encoding utf8`,
			// 考题：要求修改版本号
			TaskPrompt: `当前目录下有一个 config.json。请你使用 edit_file 工具，将其中的 version 从 v1.0.0 改为 v2.0.0。不要做其他多余操作。`,
			// 判卷脚本：检查文件是否包含 v2.0.0
			ValidateScript: `Select-String -Pattern '"version": "v2.0.0"' -Path config.json`,
		},
		{
			ID:   "test_002_code_gen",
			Name: "测试代码阅读与创建新文件的综合能力",
			// 准备靶机：生成一个简单的乘法函数
			SetupScript: `"` + "package mymath`n`nfunc Multiply(a, b int) int {`n`treturn a * b`n" + `" | Out-File -FilePath math.go -Encoding utf8`,
			// 考题：要求 Agent 根据刚才的代码，自己去写一份单元测试
			TaskPrompt: `当前目录下有一个 math.go。请你仔细阅读它，然后在同级目录下，帮我写一个规范的单元测试文件 math_test.go，用来测试 Multiply 函数。请务必包含正常的测试用例。`,
			// 判卷脚本：直接运行 go test！如果不通过则直接 0 分。
			ValidateScript: `go mod init bench; go test -v ./...`,
		},
	}

	runner := eval.NewBenchmarkRunner("deepseek-chat")
	runner.RunSuit(context.Background(), testCases)
}
