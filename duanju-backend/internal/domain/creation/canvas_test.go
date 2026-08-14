// 画布执行引擎测试（S2-Q01/Q02）：拓扑排序 + 环检测 + 节点执行器。
package creation

import "testing"

func TestTopoSortLinear(t *testing.T) {
	order, err := TopoSort([]string{"a", "b", "c"}, [][2]string{{"a", "b"}, {"b", "c"}})
	if err != nil {
		t.Fatalf("线性 DAG 不应报错: %v", err)
	}
	if len(order) != 3 || order[0] != "a" || order[2] != "c" {
		t.Errorf("拓扑序错误: %v", order)
	}
}

func TestTopoSortCycleDetected(t *testing.T) {
	_, err := TopoSort([]string{"a", "b", "c"}, [][2]string{{"a", "b"}, {"b", "c"}, {"c", "a"}})
	if err == nil {
		t.Error("环 DAG 应被检测拦截")
	}
}

func TestTopoSortDiamond(t *testing.T) {
	order, err := TopoSort([]string{"a", "b", "c", "d"}, [][2]string{{"a", "b"}, {"a", "c"}, {"b", "d"}, {"c", "d"}})
	if err != nil {
		t.Fatalf("菱形 DAG 不应报错: %v", err)
	}
	// a 必须在首位，d 必须在末位
	if order[0] != "a" || order[3] != "d" {
		t.Errorf("拓扑序错误: %v", order)
	}
}

func TestExecuteNodeTypes(t *testing.T) {
	for _, nt := range []string{"INPUT", "SCRIPT_SPLIT", "ASSET_GEN", "VIDEO_GEN", "EXPORT", "OUTPUT", "LLM"} {
		r, err := ExecuteNode(nt, map[string]any{})
		if err != nil {
			t.Errorf("节点 %s 执行失败: %v", nt, err)
		}
		if r["status"] != "SUCCESS" {
			t.Errorf("节点 %s 状态错误: %v", nt, r["status"])
		}
	}
	if _, err := ExecuteNode("UNKNOWN", nil); err == nil {
		t.Error("未知节点类型应报错")
	}
}