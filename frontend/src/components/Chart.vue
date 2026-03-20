<template>
  <div class="chart">
    <canvas class="chart-canvas" />
  </div>
</template>

<script>
import Chart from 'chart.js/auto';

const transitionMarkerPlugin = {
  id: 'transition-marker',
  afterDatasetsDraw(chart) {
    const transitionLabel = chart?.config?.data?.transitionLabel;
    if (!transitionLabel || chart.config.type !== 'line') {
      return;
    }

    const xScale = chart.scales?.x;
    const chartArea = chart.chartArea;
    if (!xScale || !chartArea) {
      return;
    }

    const x = xScale.getPixelForValue(transitionLabel);
    if (!Number.isFinite(x)) {
      return;
    }

    const { ctx } = chart;
    ctx.save();
    ctx.setLineDash([6, 6]);
    ctx.strokeStyle = 'rgba(0, 85, 212, 0.35)';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(x, chartArea.top);
    ctx.lineTo(x, chartArea.bottom);
    ctx.stroke();
    ctx.setLineDash([]);

    ctx.fillStyle = 'rgba(0, 85, 212, 0.8)';
    ctx.font = '12px sans-serif';
    ctx.textAlign = 'left';
    ctx.textBaseline = 'top';
    ctx.fillText('Hourly detail', Math.min(x + 8, chartArea.right - 80), chartArea.top + 6);
    ctx.restore();
  },
};

Chart.register(transitionMarkerPlugin);

const DEFAULT_DONUT = {
  type: 'doughnut',
  data: {},
  options: {
    responsive: true,
    cutout: '70%',
    maintainAspectRatio: false,
    plugins: {
      legend: {
        display: false,
      },
      tooltip: {
        backgroundColor: '#fff',
        borderColor: '#ddd',
        borderWidth: 1,
        titleColor: '#666',
        bodyColor: '#666',
        bodyFont: {
          size: 15,
        },
        bodySpacing: 10,
        padding: 10,
        callbacks: {
          label: (item) => {
            const data = item.chart.data.datasets[item.datasetIndex];
            const total = data.data.reduce((acc, val) => acc + val, 0);
            const val = data.data[item.dataIndex];
            const percentage = ((val / total) * 100).toFixed(2);
            return `${val} (${percentage}%)`;
          },
        },
      },
    },
  },
};

const DEFAULT_LINE = {
  type: 'line',
  data: {},
  options: {
    responsive: true,
    lineTension: 0.5,
    maintainAspectRatio: false,
    interaction: {
      intersect: false,
      axis: 'index',
    },
    plugins: {
      legend: {
        display: false,
      },
      tooltip: {
        backgroundColor: '#fff',
        borderColor: '#ddd',
        borderWidth: 1,
        bodyColor: '#666',
        displayColors: true,
        bodyFont: {
          size: 15,
        },
        bodySpacing: 10,
        padding: 10,
      },
    },
    scales: {
      x: {
        grid: {
          display: false,
        },
        ticks: {
          maxRotation: 0,
          autoSkip: true,
        },
      },
      y: {
        grid: {
          display: false,
        },
        ticks: {
          precision: 0,
        },
      },
    },
  },
};

const DEFAULT_BAR = {
  type: 'bar',
  data: {},
  options: {
    responsive: true,
    indexAxis: 'y',
    barThickness: 40,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        display: false,
      },
      tooltip: {
        backgroundColor: '#fff',
        borderColor: '#ddd',
        borderWidth: 1,
        titleColor: '#666',
        bodyColor: '#666',
        bodyFont: {
          size: 15,
        },
        bodySpacing: 10,
        padding: 10,
      },
    },
    scales: {
      x: {
        grid: {
          display: false,
        },
      },
      y: {
        grid: {
          display: false,
        },
      },
    },
  },
};

export default {
  name: 'Chart',

  props: {
    data: { type: Object, default: () => { } },
    type: { type: String, default: 'line' },
    onClick: { type: Function, default: () => { } },
  },

  mounted() {
    const ctx = this.$el.querySelector('.chart-canvas');

    let def = {};
    switch (this.$props.type) {
      case 'donut':
        def = DEFAULT_DONUT;
        break;
      case 'bar':
        def = DEFAULT_BAR;
        break;
      default:
        def = DEFAULT_LINE;
        break;
    }

    const conf = { ...def, data: this.$props.data };
    if (this.$props.onClick) {
      conf.options.onClick = this.$props.onClick;
    }
    this.chart = new Chart(ctx, conf);
  },
};
</script>
