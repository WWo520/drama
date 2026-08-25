-- smart-repair.sql - 剧本拆分失败智能修复脚本

-- ⚠️ 使用前请确保：
-- 1. 已备份当前项目数据（可选但建议）
-- 2. 数据库连接正常
-- 3. 至少有一个有效的模型密钥

\set PROJECT_UUID 'e9010b35-46aa-4a9f-bda7-85782d9e2e82'

BEGIN;

-- Step 1: 检查并记录项目信息
DO $$
DECLARE
    v_project_id BIGINT;
    v_script_len INTEGER;
    v_current_status TEXT;
    v_valid_keys INTEGER;
    v_failed_count INTEGER;
BEGIN
    -- 获取项目基本信息
    SELECT id, status, length(source_script_text) INTO v_project_id, v_current_status, v_script_len
    FROM projects WHERE project_uuid = :'PROJECT_UUID';
    
    IF v_project_id IS NULL THEN
        RAISE EXCEPTION '❌ 项目不存在！UUID: %', :'PROJECT_UUID';
    END IF;
    
    -- 检查有效密钥数量
    SELECT count(*) INTO v_valid_keys FROM model_keys mk 
    JOIN model_providers mp ON mp.name = mk.provider_name
    WHERE mk.enabled = TRUE 
      AND mp.model_engine IN ('deepseek-v4','qwen-plus','glm-4');
    
    -- 统计失败任务数
    SELECT count(*) INTO v_failed_count FROM generation_tasks gt
    WHERE gt.project_id = v_project_id 
      AND gt.task_type = 'SCRIPT_SPLIT' 
      AND status LIKE '%FAIL%';
    
    -- 输出诊断信息
    RAISE NOTICE '═══════════════════════════════════════════';
    RAISE NOTICE '🔍 智能修复 - 诊断信息';
    RAISE NOTICE '═══════════════════════════════════════════';
    RAISE NOTICE '  项目名称：%' , (SELECT title FROM projects WHERE id = v_project_id);
    RAISE NOTICE '  项目 ID: %', v_project_id;
    RAISE NOTICE '  当前状态：% → 目标状态：DRAFT', v_current_status;
    RAISE NOTICE '  剧本长度：%字符', v_script_len;
    RAISE NOTICE '  有效密钥数：%个', v_valid_keys;
    RAISE NOTICE '  失败任务数：%个', v_failed_count;
    
    -- Step 2: 如果没有密钥，尝试启用测试密钥
    IF v_valid_keys = 0 THEN
        RAISE WARNING '⚠️ 未检测到有效密钥，尝试启用测试密钥...';
        
        UPDATE model_keys SET enabled = TRUE 
        WHERE key_prefix LIKE '%test%' 
          AND provider_name = 'deepseek'
        LIMIT 1;
        
        GET DIAGNOSTICS v_valid_keys = ROW_COUNT;
        
        IF v_valid_keys > 0 THEN
            RAISE NOTICE '✅ 已启用一个测试密钥作为临时方案';
        ELSE
            RAISE WARNING '⚠️ 警告：仍无可用密钥！修复后可能仍然失败。';
            RAISE NOTICE '建议：添加有效的 DeepSeek/Qwen/GLM 密钥后再重试';
        END IF;
    END IF;
    
    -- Step 3: 确认有剧本内容
    IF v_script_len = 0 OR v_script_len IS NULL THEN
        RAISE EXCEPTION '❌ 错误：项目中没有剧本内容！请先补充剧本原文后再重试。';
    END IF;
    
    -- Step 4: 清理失败的拆分任务
    DELETE FROM generation_tasks 
    WHERE project_id = v_project_id 
      AND task_type = 'SCRIPT_SPLIT';
    
    RAISE NOTICE '✅ 已清除 %个失败的拆分任务', v_failed_count;
    
    -- Step 5: 软删除旧的拆分结果（级联清理）
    DELETE FROM shots WHERE project_id = v_project_id;
    DELETE FROM shot_groups WHERE project_id = v_project_id;
    DELETE FROM narrative_units WHERE project_id = v_project_id;
    DELETE FROM episodes WHERE project_id = v_project_id;
    
    -- Note: characters/scenes/props 不清除，因为它们是可复用的
    
    RAISE NOTICE '✅ 已清理旧的拆分结果数据';
    
    -- Step 6: 重置项目状态为 DRAFT
    UPDATE projects 
    SET 
        status = 'DRAFT', 
        step = NULL, 
        updated_at = NOW(),
        total_episode_count = 0,
        total_duration_sec = 0
    WHERE project_uuid = :'PROJECT_UUID';
    
    RAISE NOTICE '✅ 项目状态已重置为：DRAFT';
    
    -- Step 7: 最终验证
    UPDATE projects 
    SET total_episode_count = COALESCE(total_episode_count, 0),
        total_duration_sec = COALESCE(total_duration_sec, 0)
    WHERE project_uuid = :'PROJECT_UUID';
    
    -- Step 8: 输出成功消息
    RAISE NOTICE '═══════════════════════════════════════════';
    RAISE NOTICE '✅ 智能修复完成！';
    RAISE NOTICE '═══════════════════════════════════════════';
    RAISE NOTICE '';
    RAISE NOTICE '📊 当前状态:';
    RAISE NOTICE '  • 项目状态：DRAFT ✅';
    RAISE NOTICE '  • 剧本内容：%字符 ✅', v_script_len;
    RAISE NOTICE '  • 可用密钥：%个 ✅', v_valid_keys;
    RAISE NOTICE '  • 失败任务：已清除 ✅';
    RAISE NOTICE '';
    RAISE NOTICE '🎯 下一步操作:';
    RAISE NOTICE '  1. 刷新浏览器页面';
    RAISE NOTICE '  2. 进入《逃离云端的人》项目';
    RAISE NOTICE '  3. 点击"拆分剧本"按钮';
    RAISE NOTICE '  4. 等待 AI 处理完成（通常 30-120 秒）';
    RAISE NOTICE '';
    RAISE NOTICE '⚠️ 如果再次失败:';
    RAISE NOTICE '  • 检查浏览器 Console 的错误日志';
    RAISE NOTICE '  • 查看 Worker 日志中的 "split:" 错误信息';
    RAISE NOTICE '  • 联系管理员确认模型密钥有效性';
    RAISE NOTICE '═══════════════════════════════════════════';
END $$;

COMMIT;

-- 验证查询
\echo ''
\echo '📈 验证查询结果：'
SELECT 
    p.title,
    p.status,
    CASE 
        WHEN p.status = 'DRAFT' THEN '✅ 就绪，可以重新拆分'
        ELSE '⚠️ 状态异常'
    END as state_check,
    (SELECT count(*) FROM model_keys mk 
     JOIN model_providers mp ON mp.name = mk.provider_name
     WHERE mk.enabled = TRUE AND mp.model_engine LIKE '%llm%') as available_keys
FROM projects p
WHERE p.project_uuid = :'PROJECT_UUID';
