(function () {
  'use strict';

  const REFRESH_INTERVAL_MS = 15000;
  const key = new URLSearchParams(window.location.search).get('key');
  const refreshButton = document.getElementById('refreshButton');
  const todayMetrics = document.getElementById('todayMetrics');
  const updatedAt = document.getElementById('updatedAt');
  const chartElement = document.getElementById('usageChart');
  const modelTokenChartElement = document.getElementById('modelTokenChart');
  const modelCostChartElement = document.getElementById('modelCostChart');

  const MODEL_COLORS = [
    '#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6',
    '#06b6d4', '#ec4899', '#84cc16', '#f97316', '#6366f1',
    '#14b8a6', '#a855f7', '#eab308', '#22c55e', '#0ea5e9'
  ];

  let loading = false;
  let usageChart = null;
  let modelTokenChart = null;
  let modelCostChart = null;
  let latestTrend = [];
  let latestModelUsage = [];

  if (!key) {
    window.location.replace('/key-usage');
    return;
  }

  function number(value, maximumFractionDigits) {
    return new Intl.NumberFormat('zh-CN', {
      maximumFractionDigits: maximumFractionDigits === undefined ? 0 : maximumFractionDigits
    }).format(Number(value) || 0);
  }

  function currency(value) {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 2,
      maximumFractionDigits: 6
    }).format(Number(value) || 0);
  }

  function costMetric(today, quota) {
    const limit = Number(quota.limit_usd);
    const percentage = Number(quota.usage_percentage);
    if (Number.isFinite(limit) && limit > 0 && Number.isFinite(percentage)) {
      return {
        label: '实际费用',
        value: currency(today.effective_cost),
        detail: `费用上限 ${currency(limit)} · 已使用 ${number(percentage, 1)}%`,
        progress: Math.max(0, Math.min(100, percentage)),
        cost: true
      };
    }
    return {
      label: '实际费用',
      value: currency(today.effective_cost),
      cost: true
    };
  }

  function renderMetrics(data) {
    const today = data.today || {};
    const quota = data.cost_quota || {};
    const metrics = [
      costMetric(today, quota),
      { label: '请求次数', value: number(today.request_count) },
      { label: '总 Token', value: number(today.total_tokens) }
    ];

    const fragment = document.createDocumentFragment();
    metrics.forEach((metric) => {
      const item = document.createElement('div');
      item.className = `public-usage-metric${metric.cost ? ' public-usage-metric--cost' : ''}`;
      const label = document.createElement('dt');
      const value = document.createElement('dd');
      label.textContent = metric.label;
      value.textContent = metric.value;
      item.append(label, value);

      if (metric.detail) {
        const detail = document.createElement('p');
        detail.className = 'public-usage-metric-detail';
        detail.textContent = metric.detail;
        item.appendChild(detail);
      }
      if (metric.progress !== undefined) {
        const track = document.createElement('div');
        const fill = document.createElement('span');
        track.className = 'public-usage-quota-track';
        fill.className = 'public-usage-quota-fill';
        fill.style.width = `${metric.progress}%`;
        track.appendChild(fill);
        item.appendChild(track);
      }
      fragment.appendChild(item);
    });
    todayMetrics.replaceChildren(fragment);
  }

  function chartColors() {
    const styles = getComputedStyle(document.documentElement);
    return {
      cost: styles.getPropertyValue('--warning-500').trim() || '#f59e0b',
      token: styles.getPropertyValue('--primary-400').trim() || '#3ca3ff',
      text: styles.getPropertyValue('--neutral-600').trim() || '#d1d5db',
      strongText: styles.getPropertyValue('--neutral-900').trim() || '#f9fafb',
      grid: styles.getPropertyValue('--surface-border').trim() || 'rgba(255,255,255,0.12)',
      surface: styles.getPropertyValue('--surface-bg').trim() || '#ffffff',
      tooltip: styles.getPropertyValue('--surface-bg-strong').trim() || '#ffffff'
    };
  }

  function truncateLabel(value, maxLength) {
    const characters = Array.from(String(value));
    return characters.length > maxLength
      ? `${characters.slice(0, maxLength).join('')}…`
      : characters.join('');
  }

  function buildModelColorMap(rows) {
    const names = Array.from(new Set(rows.map((row) => String(row.model || '未知模型'))));
    names.sort((left, right) => left.localeCompare(right, 'zh-CN'));
    return new Map(names.map((name, index) => [name, MODEL_COLORS[index % MODEL_COLORS.length]]));
  }

  function renderModelPie(chart, element, rows, colorMap, config) {
    if (!window.echarts || !element) return chart;
    if (!chart) chart = window.echarts.init(element);

    const colors = chartColors();
    const data = rows
      .map((row) => ({
        name: String(row.model || '未知模型'),
        value: Number(row[config.field]) || 0
      }))
      .filter((item) => Number.isFinite(item.value) && item.value > 0)
      .sort((left, right) => right.value - left.value)
      .map((item) => ({
        ...item,
        itemStyle: { color: colorMap.get(item.name) }
      }));

    if (data.length === 0) {
      chart.clear();
      chart.setOption({
        animation: false,
        title: {
          text: '暂无用量数据',
          left: 'center',
          top: 'middle',
          textStyle: { color: colors.text, fontSize: 13, fontWeight: 400 }
        }
      }, true);
      return chart;
    }

    const total = data.reduce((sum, item) => sum + item.value, 0);
    const valuesByName = new Map(data.map((item) => [item.name, item.value]));
    chart.setOption({
      animationDuration: 250,
      animationDurationUpdate: 250,
      aria: { enabled: true },
      title: {
        text: config.formatTotal(total),
        subtext: config.totalLabel,
        left: 'center',
        top: '32%',
        textStyle: { color: colors.strongText, fontSize: 16, fontWeight: 600 },
        subtextStyle: { color: colors.text, fontSize: 11, lineHeight: 18 }
      },
      tooltip: {
        trigger: 'item',
        renderMode: 'richText',
        backgroundColor: colors.tooltip,
        borderColor: colors.grid,
        textStyle: { color: colors.strongText, fontSize: 12 },
        formatter(params) {
          return `${params.name}\n${config.valueLabel}: ${config.formatValue(params.value)}\n占比: ${number(params.percent, 1)}%`;
        }
      },
      legend: {
        type: 'scroll',
        orient: 'horizontal',
        left: 12,
        right: 12,
        bottom: 8,
        itemWidth: 10,
        itemHeight: 10,
        textStyle: { color: colors.text, fontSize: 11 },
        pageIconColor: colors.text,
        pageIconInactiveColor: colors.grid,
        pageTextStyle: { color: colors.text },
        formatter(name) {
          const value = valuesByName.get(name) || 0;
          const percentage = total > 0 ? value * 100 / total : 0;
          return `${truncateLabel(name, 16)}  ${config.formatValue(value)}  ${number(percentage, 1)}%`;
        }
      },
      series: [{
        name: config.valueLabel,
        type: 'pie',
        radius: ['45%', '70%'],
        center: ['50%', '40%'],
        avoidLabelOverlap: true,
        stillShowZeroSum: false,
        itemStyle: {
          borderRadius: 4,
          borderColor: colors.surface,
          borderWidth: 2
        },
        label: {
          show: true,
          position: 'inside',
          color: '#ffffff',
          fontSize: 11,
          fontWeight: 600,
          textBorderColor: 'rgba(0, 0, 0, 0.35)',
          textBorderWidth: 2,
          formatter: (params) => params.percent >= 7 ? `${number(params.percent, 0)}%` : ''
        },
        labelLine: { show: false },
        emphasis: { scaleSize: 4 },
        data
      }]
    }, true);
    return chart;
  }

  function renderModelCharts(rows) {
    const usage = Array.isArray(rows) ? rows : [];
    const colorMap = buildModelColorMap(usage);
    modelTokenChart = renderModelPie(modelTokenChart, modelTokenChartElement, usage, colorMap, {
      field: 'total_tokens',
      valueLabel: 'Token',
      totalLabel: '总 Token',
      formatValue: (value) => number(value),
      formatTotal: (value) => number(value)
    });
    modelCostChart = renderModelPie(modelCostChart, modelCostChartElement, usage, colorMap, {
      field: 'effective_cost',
      valueLabel: '费用',
      totalLabel: '总费用',
      formatValue: (value) => currency(value),
      formatTotal: (value) => currency(value)
    });
  }

  function renderChart(points) {
    if (!window.echarts || !chartElement) return;
    if (!usageChart) usageChart = window.echarts.init(chartElement);

    const colors = chartColors();
    const timeFormat = new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit' });
    const labels = points.map((point) => timeFormat.format(new Date(point.timestamp)));
    const costs = points.map((point) => Number(point.effective_cost) || 0);
    const tokens = points.map((point) => Number(point.total_tokens) || 0);

    usageChart.setOption({
      animationDuration: 250,
      color: [colors.cost, colors.token],
      grid: { left: 12, right: 12, top: 48, bottom: 8, containLabel: true },
      legend: {
        top: 4,
        data: ['费用', 'Token'],
        textStyle: { color: colors.text }
      },
      tooltip: {
        trigger: 'axis',
        formatter(params) {
          const rows = params.map((item) => {
            const value = item.seriesName === '费用' ? currency(item.value) : number(item.value);
            return `${item.marker}${item.seriesName}: ${value}`;
          });
          return `${params[0]?.axisValue || ''}<br>${rows.join('<br>')}`;
        }
      },
      xAxis: {
        type: 'category',
        boundaryGap: true,
        data: labels,
        axisLine: { lineStyle: { color: colors.grid } },
        axisTick: { show: false },
        axisLabel: { color: colors.text, hideOverlap: true }
      },
      yAxis: [
        {
          type: 'value',
          name: '费用 (USD)',
          nameTextStyle: { color: colors.text },
          axisLabel: { color: colors.text },
          splitLine: { lineStyle: { color: colors.grid } }
        },
        {
          type: 'value',
          name: 'Token',
          nameTextStyle: { color: colors.text },
          axisLabel: { color: colors.text, formatter: (value) => number(value) },
          splitLine: { show: false }
        }
      ],
      series: [
        {
          name: '费用',
          type: 'line',
          yAxisIndex: 0,
          data: costs,
          showSymbol: false,
          lineStyle: { width: 2 }
        },
        {
          name: 'Token',
          type: 'bar',
          yAxisIndex: 1,
          data: tokens,
          barMaxWidth: 18
        }
      ]
    }, true);
  }

  function render(data) {
    latestTrend = Array.isArray(data.trend) ? data.trend : [];
    latestModelUsage = Array.isArray(data.model_usage) ? data.model_usage : [];
    renderMetrics(data);
    renderModelCharts(latestModelUsage);
    renderChart(latestTrend);
    updatedAt.textContent = `更新于 ${new Intl.DateTimeFormat('zh-CN', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit'
    }).format(new Date(data.updated_at))}`;
  }

  async function load() {
    if (loading) return;
    loading = true;
    refreshButton.disabled = true;
    try {
      const query = new URLSearchParams({ key });
      const response = await fetch(`/public/key-usage?${query.toString()}`, {
        headers: { Accept: 'application/json' },
        cache: 'no-store'
      });
      if (response.status === 404) {
        window.location.replace('/key-usage');
        return;
      }
      if (!response.ok) throw new Error(`request failed: ${response.status}`);
      const payload = await response.json();
      if (!payload.success || !payload.data) throw new Error('invalid response');
      render(payload.data);
    } catch (error) {
      updatedAt.textContent = '数据暂不可用';
    } finally {
      loading = false;
      refreshButton.disabled = false;
    }
  }

  refreshButton.addEventListener('click', load);
  window.addEventListener('resize', () => {
    usageChart?.resize();
    modelTokenChart?.resize();
    modelCostChart?.resize();
  });
  window.addEventListener('ccload:themechange', () => {
    renderModelCharts(latestModelUsage);
    renderChart(latestTrend);
  });
  window.setInterval(load, REFRESH_INTERVAL_MS);
  load();
})();
