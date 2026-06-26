    // 统计数据管理
    let statsData = {
      total_requests: 0,
      success_requests: 0,
      error_requests: 0,
      active_channels: 0,
      active_models: 0,
      duration_seconds: 1,
      rpm_stats: null,
      is_today: true
    };

    // 当前选中的时间范围
    let currentTimeRange = 'today';
    let currentCustomTimeRange = null;
    const overviewAutoRefreshIntervalSeconds = 15;
    const overviewAutoRefreshState = {
      nextRefreshAt: null,
      countdownTimerId: null,
      visibilityHandler: null,
      refreshing: false,
      hasError: false,
      lastUpdatedAt: null
    };

    function buildSummaryURL() {
      const query = typeof window.buildDateRangeQuery === 'function'
        ? window.buildDateRangeQuery(currentTimeRange, currentCustomTimeRange)
        : `range=${encodeURIComponent(currentTimeRange)}`;
      return `/public/summary?${query}`;
    }

    // 加载统计数据
    async function loadStats() {
      try {
        // 添加加载状态
        document.querySelectorAll('.metric-number').forEach(el => {
          el.classList.add('animate-pulse');
        });

        const data = await fetchDataWithAuth(buildSummaryURL());
        statsData = data || statsData;
        updateStatsDisplay();

      } catch (error) {
        console.error('Failed to load stats:', error);
        showError('无法加载统计数据');
        throw error;
      } finally {
        // 移除加载状态
        document.querySelectorAll('.metric-number').forEach(el => {
          el.classList.remove('animate-pulse');
        });
      }
    }

    function getAutoRefreshStatusElement() {
      return document.getElementById('index-auto-refresh-status');
    }

    function getAutoRefreshMetaElement() {
      return document.getElementById('index-auto-refresh-meta');
    }

    function getAutoRefreshButtonElement() {
      return document.getElementById('index-auto-refresh-button');
    }

    function formatAutoRefreshText(key, replacements) {
      let text = t(key);
      if (!replacements || typeof text !== 'string') return text;
      Object.entries(replacements).forEach(([name, value]) => {
        text = text.replace(new RegExp(`\\{${name}\\}`, 'g'), String(value));
      });
      return text;
    }

    function isOverviewAutoRefreshPaused() {
      if (typeof document === 'undefined') return true;
      if (document.hidden) return true;
      if (document.querySelector('.modal.show')) return true;
      return false;
    }

    function setOverviewNextRefreshAt(seconds = overviewAutoRefreshIntervalSeconds) {
      overviewAutoRefreshState.nextRefreshAt = Date.now() + (seconds * 1000);
    }

    function formatOverviewRefreshTime(timestamp) {
      if (!timestamp) return '';
      try {
        const locale = document?.documentElement?.lang || undefined;
        return new Intl.DateTimeFormat(locale, {
          hour: '2-digit',
          minute: '2-digit',
          second: '2-digit'
        }).format(new Date(timestamp));
      } catch (_) {
        return new Date(timestamp).toLocaleTimeString();
      }
    }

    function renderOverviewAutoRefreshMeta() {
      const el = getAutoRefreshMetaElement();
      if (!el) return;
      if (overviewAutoRefreshState.lastUpdatedAt) {
        el.textContent = formatAutoRefreshText('index.autoRefresh.lastUpdated', {
          time: formatOverviewRefreshTime(overviewAutoRefreshState.lastUpdatedAt)
        });
        return;
      }
      el.textContent = formatAutoRefreshText('index.autoRefresh.neverUpdated');
    }

    function renderOverviewAutoRefreshStatus() {
      const el = getAutoRefreshStatusElement();
      if (!el) return;

      const paused = isOverviewAutoRefreshPaused();
      el.classList.remove('is-refreshing', 'is-paused', 'is-error');

      if (overviewAutoRefreshState.refreshing) {
        el.classList.add('is-refreshing');
        el.textContent = formatAutoRefreshText('index.autoRefresh.refreshing');
        return;
      }

      if (paused) {
        el.classList.add('is-paused');
        el.textContent = formatAutoRefreshText('index.autoRefresh.paused');
        return;
      }

      if (overviewAutoRefreshState.hasError) {
        el.classList.add('is-error');
      }

      const remainingSeconds = overviewAutoRefreshState.nextRefreshAt
        ? Math.max(0, Math.ceil((overviewAutoRefreshState.nextRefreshAt - Date.now()) / 1000))
        : overviewAutoRefreshIntervalSeconds;

      el.textContent = formatAutoRefreshText('index.autoRefresh.countdown', {
        seconds: remainingSeconds
      });
    }

    function updateOverviewRefreshButtonState() {
      const button = getAutoRefreshButtonElement();
      if (!button) return;
      button.disabled = overviewAutoRefreshState.refreshing;
    }

    function startOverviewAutoRefreshCountdown() {
      stopOverviewAutoRefreshCountdown();
      renderOverviewAutoRefreshStatus();
      renderOverviewAutoRefreshMeta();
      updateOverviewRefreshButtonState();
      overviewAutoRefreshState.countdownTimerId = window.setInterval(() => {
        if (!isOverviewAutoRefreshPaused() && !overviewAutoRefreshState.refreshing && !overviewAutoRefreshState.nextRefreshAt) {
          setOverviewNextRefreshAt();
        }
        renderOverviewAutoRefreshStatus();
      }, 1000);

      overviewAutoRefreshState.visibilityHandler = () => {
        if (!isOverviewAutoRefreshPaused() && !overviewAutoRefreshState.refreshing && !overviewAutoRefreshState.nextRefreshAt) {
          setOverviewNextRefreshAt();
        }
        renderOverviewAutoRefreshStatus();
      };
      document.addEventListener('visibilitychange', overviewAutoRefreshState.visibilityHandler);
    }

    function stopOverviewAutoRefreshCountdown() {
      if (overviewAutoRefreshState.countdownTimerId !== null) {
        clearInterval(overviewAutoRefreshState.countdownTimerId);
        overviewAutoRefreshState.countdownTimerId = null;
      }
      if (overviewAutoRefreshState.visibilityHandler) {
        document.removeEventListener('visibilitychange', overviewAutoRefreshState.visibilityHandler);
        overviewAutoRefreshState.visibilityHandler = null;
      }
    }

    async function refreshOverviewStats() {
      if (overviewAutoRefreshState.refreshing) return;
      overviewAutoRefreshState.refreshing = true;
      overviewAutoRefreshState.hasError = false;
      overviewAutoRefreshState.nextRefreshAt = null;
      renderOverviewAutoRefreshStatus();
      updateOverviewRefreshButtonState();

      try {
        await loadStats();
        overviewAutoRefreshState.hasError = false;
        overviewAutoRefreshState.lastUpdatedAt = Date.now();
      } catch (_) {
        overviewAutoRefreshState.hasError = true;
      } finally {
        overviewAutoRefreshState.refreshing = false;
        if (!isOverviewAutoRefreshPaused()) {
          setOverviewNextRefreshAt();
        }
        renderOverviewAutoRefreshStatus();
        renderOverviewAutoRefreshMeta();
        updateOverviewRefreshButtonState();
      }
    }

    // 更新统计显示
    function updateStatsDisplay() {
      const successRate = statsData.total_requests > 0
        ? ((statsData.success_requests / statsData.total_requests) * 100).toFixed(1)
        : '0.0';

      // 更新总体数字显示（成功/失败合并显示）
      document.getElementById('success-requests').textContent = formatNumber(statsData.success_requests || 0);
      document.getElementById('error-requests').textContent = formatNumber(statsData.error_requests || 0);
      document.getElementById('success-rate').textContent = successRate + '%';

      // 更新 RPM（使用峰值/平均/最近格式）
      const rpmStats = statsData.rpm_stats || null;
      const isToday = statsData.is_today !== false;
      updateGlobalRpmDisplay('total-rpm', rpmStats, isToday);

      // 更新按渠道类型统计
      if (statsData.by_type) {
        updateTypeStats('anthropic', statsData.by_type.anthropic);
        updateTypeStats('codex', statsData.by_type.codex);
        updateTypeStats('openai', statsData.by_type.openai);
        updateTypeStats('gemini', statsData.by_type.gemini);
      }
    }

    // 更新全局 RPM 显示（格式：数值 数值 数值）
    function updateGlobalRpmDisplay(elementId, stats, showRecent) {
      const el = document.getElementById(elementId);
      if (!el) return;

      if (!stats || (stats.peak_rpm < 0.01 && stats.avg_rpm < 0.01)) {
        el.innerHTML = '--';
        return;
      }

      const fmt = v => v >= 1000 ? (v / 1000).toFixed(1) + 'K' : v.toFixed(1);
      const parts = [];

      if (stats.peak_rpm >= 0.01) {
        parts.push(`<span style="color:${getRpmColor(stats.peak_rpm)}">${fmt(stats.peak_rpm)}</span>`);
      }
      if (stats.avg_rpm >= 0.01) {
        parts.push(`<span style="color:${getRpmColor(stats.avg_rpm)}">${fmt(stats.avg_rpm)}</span>`);
      }
      if (showRecent && stats.recent_rpm >= 0.01) {
        parts.push(`<span style="color:${getRpmColor(stats.recent_rpm)}">${fmt(stats.recent_rpm)}</span>`);
      }

      el.innerHTML = parts.length > 0 ? parts.join(' ') : '--';
    }

    // 更新单个渠道类型的统计
    function updateTypeStats(type, data) {
      // 始终显示所有卡片，保持界面完整性
      const card = document.getElementById(`type-${type}-card`);
      if (card) card.style.display = 'block';

      // 如果没有数据，显示默认值
      const totalRequests = data ? (data.total_requests || 0) : 0;
      const successRequests = data ? (data.success_requests || 0) : 0;
      const errorRequests = data ? (data.error_requests || 0) : 0;

      const successRate = totalRequests > 0
        ? ((successRequests / totalRequests) * 100).toFixed(1)
        : '0.0';

      // 更新基础统计（总请求、成功、失败、成功率）
      document.getElementById(`type-${type}-requests`).textContent = formatNumber(totalRequests);
      document.getElementById(`type-${type}-success`).textContent = formatNumber(successRequests);
      document.getElementById(`type-${type}-error`).textContent = formatNumber(errorRequests);
      document.getElementById(`type-${type}-rate`).textContent = successRate + '%';

      // 所有渠道类型的Token和成本统计
      const inputTokens = data ? (data.total_input_tokens || 0) : 0;
      const outputTokens = data ? (data.total_output_tokens || 0) : 0;
      const totalCost = data ? (data.total_cost || 0) : 0;
      const effectiveCost = data && data.effective_cost !== undefined && data.effective_cost !== null
        ? Number(data.effective_cost) || 0
        : totalCost;

      document.getElementById(`type-${type}-input`).textContent = formatNumber(inputTokens);
      document.getElementById(`type-${type}-output`).textContent = formatNumber(outputTokens);
      document.getElementById(`type-${type}-cost`).innerHTML = buildCostStackHtml(totalCost, effectiveCost, { tone: 'warning', inline: true });

      // Claude和Codex类型的缓存统计（缓存读+缓存创建）
      if (type === 'anthropic' || type === 'codex') {
        const cacheReadTokens = data ? (data.total_cache_read_tokens || 0) : 0;
        const cacheCreateTokens = data ? (data.total_cache_creation_tokens || 0) : 0;
        document.getElementById(`type-${type}-cache-read`).textContent = formatNumber(cacheReadTokens);
        document.getElementById(`type-${type}-cache-create`).textContent = formatNumber(cacheCreateTokens);
      }

      // OpenAI和Gemini类型的缓存统计（仅缓存读）
      if (type === 'openai' || type === 'gemini') {
        const cacheReadTokens = data ? (data.total_cache_read_tokens || 0) : 0;
        document.getElementById(`type-${type}-cache-read`).textContent = formatNumber(cacheReadTokens);
      }

      // 渲染两个饼图：今日 API 令牌消费对比 + 今日渠道消费占用
      renderTokenCostPie(`type-${type}-pie-token`, data && data.by_token);
      renderChannelCostPie(`type-${type}-pie-channel`, data && data.by_channel);
    }

    // 渲染「今日 API 令牌消费对比」饼图
    function renderTokenCostPie(containerId, byToken) {
      const items = [];
      if (Array.isArray(byToken)) {
        for (const entry of byToken) {
          const cost = Number(entry && entry.cost) || 0;
          if (cost > 0) {
            const name = (entry && entry.name && String(entry.name).trim()) || `#${entry.auth_token_id || ''}` || t('index.pies.other');
            items.push({ name, value: cost });
          }
        }
      }
      renderPie(containerId, items, '$', null);
    }

    // 渲染「今日渠道消费占用」饼图
    function renderChannelCostPie(containerId, byChannel) {
      const items = [];
      if (Array.isArray(byChannel)) {
        for (const entry of byChannel) {
          const cost = Number(entry && entry.cost) || 0;
          if (cost > 0) {
            const name = (entry.channel_name || `#${entry.channel_id || ''}`).trim() || t('index.pies.other');
            items.push({ name, value: cost });
          }
        }
      }
      renderPie(containerId, items, '$', null);
    }

    // 通用饼图渲染器（基于 echarts，复用 ui.js 的 getChartTheme）
    const pieChartInstances = {};

    function getPieChartInstance(containerId) {
      const container = document.getElementById(containerId);
      if (!container || typeof window.echarts === 'undefined') return null;

      let chart = pieChartInstances[containerId];
      const chartDom = chart && typeof chart.getDom === 'function' ? chart.getDom() : null;
      const disposed = chart && typeof chart.isDisposed === 'function' ? chart.isDisposed() : false;

      if (chart && (disposed || chartDom !== container || !chartDom?.isConnected)) {
        try { chart.dispose(); } catch (_) { /* 忽略旧实例清理异常 */ }
        delete pieChartInstances[containerId];
        chart = null;
      }

      if (!chart) {
        chart = typeof window.echarts.getInstanceByDom === 'function'
          ? window.echarts.getInstanceByDom(container)
          : null;
      }

      if (!chart) {
        chart = window.echarts.init(container);
      }

      pieChartInstances[containerId] = chart;
      return chart;
    }

    function schedulePieChartResize(chart) {
      if (!chart || typeof window.requestAnimationFrame !== 'function') return;
      window.requestAnimationFrame(() => {
        window.requestAnimationFrame(() => {
          try { chart.resize(); } catch (_) { /* 忽略重排阶段 resize 异常 */ }
        });
      });
    }

    function renderPie(containerId, items, unit, colorMap) {
      const chart = getPieChartInstance(containerId);
      if (!chart) return;
      const theme = (typeof window.getChartTheme === 'function') ? window.getChartTheme() : null;
      const mutedText = theme ? theme.mutedText : '#6b7280';
      const tooltipBg = theme ? theme.tooltipBg : 'rgba(255, 255, 255, 0.98)';
      const tooltipBorder = theme ? theme.tooltipBorder : 'rgba(17, 24, 39, 0.16)';
      const tooltipText = theme ? theme.tooltipText : '#111827';
      const surface = theme ? theme.surface : '#ffffff';

      if (!items || items.length === 0) {
        chart.clear();
        chart.setOption({
          title: {
            text: t('index.pies.noData'),
            left: 'center',
            top: 'center',
            textStyle: { color: mutedText, fontSize: 12 }
          }
        });
        schedulePieChartResize(chart);
        return;
      }

      const sorted = items.slice().sort((a, b) => b.value - a.value);
      const total = sorted.reduce((s, i) => s + i.value, 0);
      const palette = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6',
        '#06b6d4', '#ec4899', '#84cc16', '#f97316', '#6366f1',
        '#14b8a6', '#a855f7', '#eab308', '#22c55e', '#0ea5e9'];
      const colors = sorted.map((it, idx) => (colorMap && colorMap[it.name]) || palette[idx % palette.length]);

      chart.setOption({
        tooltip: {
          trigger: 'item',
          backgroundColor: tooltipBg,
          borderColor: tooltipBorder,
          textStyle: { color: tooltipText, fontSize: 12 },
          formatter: function (params) {
            const v = params.value;
            const formatted = unit === '$' ? `$${v.toFixed(2)}` : v.toLocaleString();
            return `${params.name}<br/>${formatted}<br/>${params.percent.toFixed(0)}%`;
          }
        },
        legend: {
          type: 'scroll',
          orient: 'vertical',
          right: 4,
          top: 'middle',
          textStyle: { fontSize: 10, color: mutedText },
          pageIconColor: mutedText,
          pageTextStyle: { color: mutedText },
          formatter: function (name) {
            const item = sorted.find(d => d.name === name);
            if (!item || total <= 0) return name;
            const cost = unit === '$' ? `$${Math.round(item.value)}` : Math.round(item.value).toLocaleString();
            const pct = Math.round((item.value / total) * 100);
            const maxLen = 12;
            const truncated = name.length > maxLen ? name.slice(0, maxLen) + '…' : name;
            return `${truncated} ${cost} ${pct}%`;
          }
        },
        color: colors,
        series: [{
          type: 'pie',
          radius: ['38%', '62%'],
          center: ['32%', '50%'],
          avoidLabelOverlap: true,
          itemStyle: { borderRadius: 4, borderColor: surface, borderWidth: 2 },
          label: { show: false },
          emphasis: {
            label: { show: true, fontSize: 11, fontWeight: 'bold', formatter: p => `${p.percent.toFixed(1)}%` },
            itemStyle: { shadowBlur: 8, shadowColor: 'rgba(0, 0, 0, 0.25)' }
          },
          data: sorted
        }]
      }, true);
      schedulePieChartResize(chart);
    }

    // 主题切换时刷新饼图
    window.addEventListener('ccload:themechange', () => {
      if (!statsData || !statsData.by_type) return;
      const types = ['anthropic', 'codex', 'openai', 'gemini'];
      for (const t of types) {
        renderTokenCostPie(`type-${t}-pie-token`, statsData.by_type[t] && statsData.by_type[t].by_token);
        renderChannelCostPie(`type-${t}-pie-channel`, statsData.by_type[t] && statsData.by_type[t].by_channel);
      }
    });

    // 窗口尺寸变化时重绘饼图
    window.addEventListener('resize', () => {
      Object.values(pieChartInstances).forEach(c => { if (c) c.resize(); });
    });

    // 通知系统统一由 ui.js 提供（showSuccess/showError/showNotification）

    // 注销功能（已由 ui.js 的 onLogout 统一处理）

    // 自动刷新由 createAutoRefresh 统一管理（system_settings.auto_refresh_interval_seconds）

    // 页面初始化
    window.initPageBootstrap({
      topbarKey: 'index',
      run: () => {
      window.bindTimeRangeSelector({
        containerId: 'index-time-range',
        values: ['today', 'yesterday', 'day_before_yesterday', 'this_week', 'last_week', 'this_month', 'last_month', 'custom'],
        initialValue: currentTimeRange,
        customRange: currentCustomTimeRange,
        onChange: (range, customRange) => {
          currentTimeRange = range;
          if (range === 'custom') currentCustomTimeRange = customRange;
          void refreshOverviewStats();
        }
      });

      if (window.i18n && typeof window.i18n.onLocaleChange === 'function') {
        window.i18n.onLocaleChange(() => {
          renderOverviewAutoRefreshStatus();
          renderOverviewAutoRefreshMeta();
        });
      }

      const autoRefreshButton = getAutoRefreshButtonElement();
      if (autoRefreshButton) {
        autoRefreshButton.addEventListener('click', () => {
          void refreshOverviewStats();
        });
      }

      startOverviewAutoRefreshCountdown();

      // 加载统计数据
      void refreshOverviewStats();

      if (window.CCPageLifecycle && typeof window.CCPageLifecycle.disposeCharts === 'function') {
        window.CCPageLifecycle.disposeCharts(pieChartInstances);
      }
      if (window.CCPageLifecycle && typeof window.CCPageLifecycle.onCleanup === 'function') {
        window.CCPageLifecycle.onCleanup(stopOverviewAutoRefreshCountdown);
      }

      // 概览页固定每 15 秒自动刷新一次
      if (typeof window.createAutoRefresh === 'function') {
        window.createAutoRefresh({ load: refreshOverviewStats, intervalSeconds: overviewAutoRefreshIntervalSeconds }).init();
      }

      // 添加页面动画
      document.querySelectorAll('.animate-slide-up').forEach((el, index) => {
        el.style.animationDelay = `${index * 0.1}s`;
      });
      }
    });
