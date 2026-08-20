// 批次 3 Step2 验收数据准备：注册账号 → 创建项目（自动触发拆分）→ 轮询至拆分完成
// 用法：node scripts/prepare-step2-e2e.mjs
const BASE = "http://127.0.0.1:8080/api/v1";
const EMAIL = "b3step2@test.com";
const PASSWORD = "B3step2Pass9";
const NICKNAME = "批次三验收";

const SCRIPT = `第一集 深夜惊魂
深夜，林晚拖着疲惫的身体回到公寓。她刚把钥匙插进锁孔，就发现门锁有被撬过的痕迹。她心头一紧，缓缓推开门。屋内一片漆黑，空气中弥漫着陌生的气息。她摸向墙边的开关，灯亮的瞬间，一个黑影从窗帘后闪出。林晚惊叫出声，手中的包掉落在地。黑影逼近，她抓起玄关的花瓶砸了过去。对方躲闪不及，踉跄后退。林晚趁机冲向大门，却被对方一把抓住手腕。就在千钧一发之际，走廊传来脚步声，黑影咒骂一声，翻窗而逃。

第二集 神秘邻居
警察离开后，林晚瘫坐在沙发上瑟瑟发抖。隔壁的陈默敲门送来一杯热茶。陈默是一名自由摄影师，搬来三个月，平日寡言少语。他提醒林晚，这栋楼最近发生过两起入室案件，警方一直没抓到人。林晚想起黑影逃走时留下的那枚袖扣，上面刻着一个奇怪的符号。她把袖扣拿给陈默看，陈默脸色微变，说他在楼下便利店见过同样的符号。两人决定第二天一起去便利店查探。

第三集 符号之谜
次日清晨，林晚和陈默来到便利店。店员回忆说，前几天确实有个戴帽子的男人买过水，袖口也带着那样的符号。陈默调出自己拍摄的城市影像，在一段夜拍素材里发现了那个戴帽子的男人，他总在深夜出没于不同的公寓楼下。林晚意识到，自己可能不是随机目标。当晚，林晚在家中发现了被翻动过的痕迹，桌上的笔记本电脑不见了。陈默安慰她别怕，并提议暂时住到他家客房。林晚犹豫再三，最终点了点头。`;

async function req(method, path, body, token) {
  const resp = await fetch(BASE + path, {
    method,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await resp.json();
  if (data.code !== 0) throw new Error(`${method} ${path} -> code=${data.code} msg=${data.message}`);
  return data.data;
}

async function main() {
  // 1. 注册（已存在则直接登录）
  let token;
  try {
    const reg = await req("POST", "/auth/register", { email: EMAIL, password: PASSWORD, nickname: NICKNAME });
    token = reg.access_token ?? reg.token;
    console.log("[注册成功]", EMAIL);
  } catch (e) {
    console.log("[注册跳过]", e.message);
    const login = await req("POST", "/auth/login", { email: EMAIL, password: PASSWORD });
    token = login.access_token ?? login.token;
    console.log("[登录成功]", EMAIL);
  }

  // 2. 创建项目（自动触发拆分）
  const proj = await req("POST", "/projects", { title: "深夜公寓·批次3验收", source_script_text: SCRIPT }, token);
  const projectId = proj.project_id;
  console.log("[项目已创建]", projectId, "status =", proj.status);

  // 3. 轮询拆分状态（DRAFT 时主动触发拆分；最长 5 分钟）
  let triggered = false;
  for (let i = 0; i < 60; i++) {
    await new Promise((r) => setTimeout(r, 5000));
    const p = await req("GET", `/projects/${projectId}`, null, token);
    console.log(`[轮询 ${i + 1}] status =`, p.status, "episodes =", p.total_episode_count);
    if (p.status === "DRAFT" && !triggered) {
      await req("POST", `/projects/${projectId}/split-script`, { force: false, script_text: SCRIPT }, token);
      triggered = true;
      console.log("[已触发拆分]");
      continue;
    }
    if (p.status !== "SCRIPT_SPLITTING") {
      if (p.status === "SCRIPT_SPLIT_FAILED") throw new Error("剧本拆分失败");
      if (p.total_episode_count === 0 && !triggered) continue; // 尚未生成剧集，继续等待
      console.log("[拆分完成]");
      break;
    }
  }

  // 4. 输出资产概况
  const eps = await req("GET", `/projects/${projectId}/episodes`, null, token);
  const ep = eps.items[0];
  const [chars, scenes, props, ov] = await Promise.all([
    req("GET", `/episodes/${ep.episode_id}/characters`, null, token),
    req("GET", `/episodes/${ep.episode_id}/scenes`, null, token),
    req("GET", `/episodes/${ep.episode_id}/props`, null, token),
    req("GET", `/episodes/${ep.episode_id}/assets/overview`, null, token),
  ]);
  console.log("[资产概况] characters =", chars.items.length, "scenes =", scenes.items.length, "props =", props.items.length);
  console.log("[overview]", JSON.stringify(ov));
  console.log("\n=== 验收凭据 ===");
  console.log("账号:", EMAIL);
  console.log("密码:", PASSWORD);
  console.log("项目:", projectId);
  console.log("Step2 地址: http://localhost:5174/projects/" + projectId + "/step/2");
}

main().catch((e) => {
  console.error("失败:", e.message);
  process.exit(1);
});
