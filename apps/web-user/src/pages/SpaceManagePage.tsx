// S2-F01 空间管理页：团队空间 + 成员/角色/邀请 + 团队积分/报表。
import { useCallback, useEffect, useState } from "react";

import { api, spaceApi, type MemberItem } from "@duanju/api-client";

const ROLE_OPTIONS = ["ADMIN", "EDITOR", "MEMBER", "VIEWER", "BILLING_ONLY"];

export default function SpaceManagePage() {
  const [teamName, setTeamName] = useState("");
  const [spaces, setSpaces] = useState<{ space_id: string; name: string; type: string; my_role: string }[]>([]);
  const [activeSpace, setActiveSpace] = useState<string>("");
  const [members, setMembers] = useState<MemberItem[]>([]);
  const [myRole, setMyRole] = useState("");
  const [account, setAccount] = useState<{ current_balance: number; frozen_balance: number } | null>(null);
  const [report, setReport] = useState<Record<string, unknown>[]>([]);
  const [inviteAccount, setInviteAccount] = useState("");
  const [addUserId, setAddUserId] = useState("");
  const [toast, setToast] = useState<string | null>(null);

  const showToast = useCallback((m: string) => {
    setToast(m);
    setTimeout(() => setToast(null), 3000);
  }, []);

  const loadSpaces = useCallback(async () => {
    try {
      const d = await api.get<{ items: { space_id: string; name: string; type: string; my_role: string }[] }>("/spaces");
      const teams = d.items.filter((s) => s.type === "TEAM");
      setSpaces(teams);
      if (teams.length > 0 && !activeSpace) setActiveSpace(teams[0].space_id);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "加载空间失败");
    }
  }, [activeSpace, showToast]);

  const loadTeam = useCallback(async () => {
    if (!activeSpace) return;
    try {
      const m = await spaceApi.members(activeSpace);
      setMembers(m.items);
      setMyRole(m.my_role);
      const acc = await spaceApi.teamBilling(activeSpace);
      setAccount(acc);
      const rep = await spaceApi.teamReport(activeSpace);
      setReport(rep.items);
    } catch {
      /* 忽略 */
    }
  }, [activeSpace]);

  useEffect(() => {
    void loadSpaces();
  }, [loadSpaces]);
  useEffect(() => {
    void loadTeam();
  }, [loadTeam]);

  const createTeam = async () => {
    if (!teamName.trim()) return;
    try {
      const t = await spaceApi.createTeam(teamName.trim());
      showToast("团队空间已创建");
      setTeamName("");
      setActiveSpace(t.space_id);
      void loadSpaces();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "创建失败");
    }
  };

  const addMember = async () => {
    if (!addUserId.trim() || !activeSpace) return;
    try {
      await spaceApi.addMember(activeSpace, addUserId.trim());
      showToast("成员已添加");
      setAddUserId("");
      void loadTeam();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "添加失败");
    }
  };

  const invite = async () => {
    if (!inviteAccount.trim() || !activeSpace) return;
    try {
      const r = await spaceApi.invite(activeSpace, inviteAccount.trim());
      showToast(`邀请链接已生成：${r.invite_url}`);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "邀请失败");
    }
  };

  const recharge = async () => {
    if (!activeSpace) return;
    try {
      const r = await spaceApi.teamRecharge(activeSpace, {
        package_code: "TEAM_PKG_M",
        amount_yuan: "99.00",
        points: 1000,
      });
      await spaceApi.teamRechargePay(r.order_no);
      showToast("团队积分充值到账 +1000");
      void loadTeam();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "充值失败");
    }
  };

  const canManage = myRole === "OWNER" || myRole === "ADMIN";

  return (
    <div className="min-h-screen bg-[#0B0B1A] p-6 text-slate-100">
      <h1 className="text-lg font-semibold">空间管理</h1>

      {/* 创建团队 */}
      <div className="mt-4 flex items-center gap-3 rounded-2xl border border-white/5 bg-white/[0.03] p-4">
        <input
          value={teamName}
          onChange={(e) => setTeamName(e.target.value)}
          placeholder="团队空间名称"
          className="flex-1 rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm outline-none placeholder:text-slate-600"
        />
        <button
          onClick={() => void createTeam()}
          className="rounded-lg bg-gradient-to-r from-amber-400 to-amber-500 px-5 py-2 text-sm font-semibold text-black"
        >
          创建团队
        </button>
      </div>

      {/* 团队选择 */}
      <div className="mt-4 flex items-center gap-2">
        <span className="text-xs text-slate-500">团队：</span>
        {spaces.map((s) => (
          <button
            key={s.space_id}
            onClick={() => setActiveSpace(s.space_id)}
            className={`rounded-full px-3 py-1 text-xs ${
              activeSpace === s.space_id ? "bg-amber-400/20 text-amber-300" : "text-slate-400"
            }`}
          >
            {s.name}
          </button>
        ))}
      </div>

      {activeSpace && (
        <div className="mt-4 grid grid-cols-2 gap-4">
          {/* 成员管理 */}
          <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-4">
            <h2 className="text-sm font-semibold">成员（{myRole}）</h2>
            <div className="mt-3 space-y-2">
              {members.map((m) => (
                <div key={m.user_id} className="flex items-center justify-between rounded-lg bg-white/[0.03] px-3 py-2">
                  <span className="text-sm">{m.nickname}</span>
                  <div className="flex items-center gap-2">
                    {canManage && m.role !== "OWNER" ? (
                      <select
                        value={m.role}
                        onChange={(e) =>
                          void spaceApi
                            .updateRole(activeSpace, m.user_uuid, e.target.value)
                            .then(() => void loadTeam())
                        }
                        className="rounded border border-white/10 bg-black/40 px-2 py-1 text-xs"
                      >
                        {ROLE_OPTIONS.map((r) => (
                          <option key={r} value={r}>
                            {r}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <span className="text-xs text-slate-400">{m.role}</span>
                    )}
                    {canManage && m.role !== "OWNER" && (
                      <button
                        onClick={() => void spaceApi.removeMember(activeSpace, m.user_uuid).then(() => void loadTeam())}
                        className="text-xs text-red-400"
                      >
                        移除
                      </button>
                    )}
                  </div>
                </div>
              ))}
            </div>
            {canManage && (
              <div className="mt-3 flex gap-2">
                <input
                  value={addUserId}
                  onChange={(e) => setAddUserId(e.target.value)}
                  placeholder="成员 user_id（UUID）"
                  className="flex-1 rounded-lg border border-white/10 bg-white/5 px-3 py-1.5 text-xs"
                />
                <button onClick={() => void addMember()} className="rounded-lg bg-white/10 px-3 py-1.5 text-xs">
                  添加
                </button>
              </div>
            )}
            {canManage && (
              <div className="mt-2 flex gap-2">
                <input
                  value={inviteAccount}
                  onChange={(e) => setInviteAccount(e.target.value)}
                  placeholder="邀请邮箱"
                  className="flex-1 rounded-lg border border-white/10 bg-white/5 px-3 py-1.5 text-xs"
                />
                <button onClick={() => void invite()} className="rounded-lg bg-white/10 px-3 py-1.5 text-xs">
                  生成邀请
                </button>
              </div>
            )}
          </div>

          {/* 团队积分 */}
          <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-4">
            <h2 className="text-sm font-semibold">团队积分</h2>
            <div className="mt-3 flex items-end gap-2">
              <span className="text-3xl font-bold text-amber-300">{account?.current_balance ?? 0}</span>
              <span className="text-sm text-slate-500">分（冻结 {account?.frozen_balance ?? 0}）</span>
            </div>
            {canManage && (
              <button
                onClick={() => void recharge()}
                className="mt-3 rounded-lg bg-gradient-to-r from-amber-400 to-amber-500 px-4 py-2 text-xs font-semibold text-black"
              >
                团队充值 +1000（开发）
              </button>
            )}
            <h3 className="mt-4 text-xs font-semibold text-slate-400">成员消费报表</h3>
            <div className="mt-2 space-y-1">
              {report.length === 0 && <div className="text-xs text-slate-600">暂无消费记录</div>}
              {report.map((r, i) => (
                <div key={i} className="flex justify-between text-xs">
                  <span>{String(r.nickname)}</span>
                  <span className="text-amber-300">-{String(r.total_spent)} 分（{String(r.txn_count)} 笔）</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {toast && (
        <div className="fixed bottom-6 left-1/2 -translate-x-1/2 rounded-lg border border-amber-400/30 bg-[#1a1a35] px-5 py-2.5 text-sm text-amber-200">
          {toast}
        </div>
      )}
    </div>
  );
}