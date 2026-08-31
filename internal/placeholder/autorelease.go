// 自动释放策略（auto-release）：把「磁盘水位 + 文件年龄」变成占位符。
//
// release/hydrate 是手动操作；本文件在其上叠加策略层：磁盘剩余空间低于
// 阈值时，把修改时间早于 age_days 的本地文件按「最旧优先」批量释放为
// 占位符，直到预计释放量补足缺口（或候选耗尽）。本地删除不传播（(?d)
// 语义），对端内容保留——这正是"NAS 中转 + 本地按需"的核心价值。
//
// 目录结构（internal/placeholder/autorelease.go = 策略与计划，纯逻辑可测）：
//   - planAutoRelease  计算每个文件夹要释放哪些文件（不动磁盘）
//   - Sweep            执行计划：写规则 → 删本地实体 → 每文件夹一次 Scan
package placeholder

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/suantea/asuan-keeper/internal/config"
)

// AutoReleaseOptions 一次自动释放的参数。
type AutoReleaseOptions struct {
	MinFreeBytes uint64        // 磁盘剩余低于该值触发释放
	Age          time.Duration // 只释放修改时间早于该时长的文件
}

// SweepResult 单个文件夹的执行/计划结果。
type SweepResult struct {
	FolderID   string
	FolderPath string
	Released   int    // 计划释放的文件数（执行后为实际数）
	FreedBytes uint64 // 计划释放的字节数
	Skipped    string // 非空 = 本轮跳过原因（水位充足 / 策略不符 / 已是文件夹级释放）
	Err        error
}

// releasePlan 一个文件夹的释放计划（planAutoRelease 产出，Sweep 执行）。
type releasePlan struct {
	folderID   string
	folderPath string
	// relPaths 为本地待删除的相对路径（"/" 分隔）；rules 与之一一对应。
	relPaths []string
	rules    []string
	bytes    uint64
}

// scanFunc 触发文件夹重扫（执行阶段调用；测试注入空实现）。
type scanFunc func(folderID string) error

// planAutoRelease 计算自动释放计划（纯逻辑，不改任何磁盘/引擎状态）。
// freeSpace 注入磁盘剩余空间查询，便于测试。
func planAutoRelease(cfg *config.Config, opts AutoReleaseOptions, freeSpace func(dir string) (uint64, error)) ([]releasePlan, []SweepResult) {
	var plans []releasePlan
	var results []SweepResult
	now := time.Now()

	for i := range cfg.Folders {
		f := &cfg.Folders[i]
		res := SweepResult{FolderID: f.ID, FolderPath: f.Path}
		if f.Policy != config.PolicySync {
			res.Skipped = fmt.Sprintf("策略为 %s，仅 sync 可释放", f.Policy)
			results = append(results, res)
			continue
		}
		// 文件夹级已释放（(?d)*）则本地本就应为空，无自动释放可言。
		if released, err := IsReleased(f.Path); err == nil && released {
			res.Skipped = "文件夹已处于释放状态"
			results = append(results, res)
			continue
		}
		free, err := freeSpace(f.Path)
		if err != nil {
			res.Err = fmt.Errorf("查询磁盘剩余空间失败: %w", err)
			results = append(results, res)
			continue
		}
		if free >= opts.MinFreeBytes {
			res.Skipped = "磁盘水位充足"
			results = append(results, res)
			continue
		}
		target := opts.MinFreeBytes - free

		plan, err := buildPlan(f.Path, f.ID, target, opts.Age, now)
		if err != nil {
			res.Err = err
			results = append(results, res)
			continue
		}
		if plan == nil {
			res.Skipped = "没有满足年龄条件的可释放文件"
			results = append(results, res)
			continue
		}
		res.Released = len(plan.relPaths)
		res.FreedBytes = plan.bytes
		results = append(results, res)
		plans = append(plans, *plan)
	}
	return plans, results
}

// buildPlan 选出某文件夹内要释放的文件：mtime 早于 age、未被现有规则
// 释放、跳过引擎内部条目；最旧优先，累计到补足 target 为止。
func buildPlan(folderPath, folderID string, target uint64, age time.Duration, now time.Time) (*releasePlan, error) {
	rules, err := ReadStIgnore(folderPath)
	if err != nil {
		return nil, fmt.Errorf("读取 .stignore 失败: %w", err)
	}
	released := releasedPathChecker(rules)

	cutoff := now.Add(-age)
	type candidate struct {
		rel  string
		size uint64
		mod  time.Time
	}
	var cands []candidate
	err = filepath.WalkDir(folderPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 读不到的条目跳过（权限/竞态），不中断整轮
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".stfolder" || name == ".stversions" {
				return fs.SkipDir
			}
			return nil
		}
		if name == StIgnoreFile {
			return nil
		}
		rel, rerr := filepath.Rel(folderPath, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if released(rel) {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		if info.ModTime().After(cutoff) {
			return nil // 还在年龄窗口内，不动
		}
		cands = append(cands, candidate{rel: rel, size: uint64(info.Size()), mod: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("遍历文件夹失败: %w", err)
	}

	// 最旧优先。
	sort.Slice(cands, func(a, b int) bool { return cands[a].mod.Before(cands[b].mod) })

	plan := &releasePlan{folderID: folderID, folderPath: folderPath}
	var freed uint64
	for _, c := range cands {
		if freed >= target {
			break
		}
		plan.relPaths = append(plan.relPaths, c.rel)
		plan.rules = append(plan.rules, "(?d)/"+c.rel)
		plan.bytes += c.size
		freed += c.size
	}
	if len(plan.relPaths) == 0 {
		return nil, nil
	}
	return plan, nil
}

// Sweep 执行自动释放：对每个有计划的文件夹「先写规则、再删本地实体、
// 最后一次 Scan」（与 Release 相同的顺序契约），返回全部文件夹的结果。
func Sweep(cfg *config.Config, opts AutoReleaseOptions, freeSpace func(dir string) (uint64, error), scan scanFunc) []SweepResult {
	plans, results := planAutoRelease(cfg, opts, freeSpace)
	byFolder := make(map[string]releasePlan, len(plans))
	for _, p := range plans {
		byFolder[p.folderID] = p
	}
	for i := range results {
		p, ok := byFolder[results[i].FolderID]
		if !ok || results[i].Err != nil {
			continue
		}
		if err := executePlan(p, scan); err != nil {
			results[i].Err = err
		}
	}
	return results
}

// executePlan 落地一个释放计划。顺序关键：规则先落盘，删除才不会传播。
func executePlan(p releasePlan, scan scanFunc) error {
	if err := AddRules(p.folderPath, p.rules); err != nil {
		return fmt.Errorf("写入 .stignore 失败: %w", err)
	}
	for _, rel := range p.relPaths {
		if err := os.Remove(filepath.Join(p.folderPath, filepath.FromSlash(rel))); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除本地文件 %s 失败: %w", rel, err)
		}
	}
	if scan != nil {
		if err := scan(p.folderID); err != nil {
			return fmt.Errorf("触发重扫失败: %w", err)
		}
	}
	return nil
}

// releasedPathChecker 把 .stignore 规则集编译成路径判断函数：
//   - "(?d)/x"  → 精确路径 x 已释放
//   - "(?d)/x/" 与 "(?d)/x/**" → 前缀 x/ 下的全部路径已释放
//   - 其余规则（含文件夹级 (?d)*）不在此处理（调用方已单独判断）
func releasedPathChecker(rules []string) func(rel string) bool {
	exact := make(map[string]bool)
	prefixes := make([]string, 0, len(rules))
	for _, r := range rules {
		p, ok := strings.CutPrefix(r, "(?d)/")
		if !ok {
			continue
		}
		switch {
		case strings.HasSuffix(p, "/**"):
			prefixes = append(prefixes, strings.TrimSuffix(p, "/**")+"/")
		case strings.HasSuffix(p, "/"):
			prefixes = append(prefixes, p)
		default:
			exact[p] = true
		}
	}
	return func(rel string) bool {
		if exact[rel] {
			return true
		}
		for _, pre := range prefixes {
			if strings.HasPrefix(rel, pre) {
				return true
			}
		}
		return false
	}
}
