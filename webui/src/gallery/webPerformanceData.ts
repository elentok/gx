import type { StatsWebPerformance } from "../generated/server"

// Canned web_performance sections of GET /v1/stats?range=7, captured from the
// web-performance and web-performance-low fixture scenarios.

export const webPerformanceFull: StatsWebPerformance = {
  hard_loads: [
    {
      metric: "lcp",
      has_threshold: true,
      count: 70,
      p75: 3764,
      status: "needs-improvement",
      change_pct: -0.4759,
    },
    {
      metric: "inp",
      has_threshold: true,
      count: 28,
      p75: 462.2,
      status: "needs-improvement",
      change_pct: 12.6767,
    },
    {
      metric: "cls",
      has_threshold: true,
      count: 28,
      p75: 0.1414,
      status: "needs-improvement",
      change_pct: 7.1212,
    },
    {
      metric: "fcp",
      has_threshold: true,
      count: 42,
      p75: 1772,
      status: "good",
      change_pct: -21.5929,
    },
    {
      metric: "app_ready",
      has_threshold: false,
      count: 42,
      p75: 2169.6,
      change_pct: 18.0157,
    },
  ],
  route_changes: [
    {
      metric: "route_ready",
      has_threshold: false,
      count: 56,
      p75: 1725,
      change_pct: -26.7609,
    },
    {
      metric: "inp",
      has_threshold: true,
      count: 28,
      p75: 418.52,
      status: "needs-improvement",
      change_pct: -4.7346,
    },
    {
      metric: "cls",
      has_threshold: true,
      count: 28,
      p75: 0.1971,
      status: "needs-improvement",
      change_pct: -14.5644,
    },
  ],
  trend: [
    {
      metric: "lcp",
      days: [
        {
          p75: 3773,
          count: 10,
        },
        {
          p75: 3689,
          count: 10,
        },
        {
          p75: 3848,
          count: 10,
        },
        {
          p75: 3764,
          count: 10,
        },
        {
          p75: 3680,
          count: 10,
        },
        {
          p75: 3839,
          count: 10,
        },
        {
          p75: 3185,
          count: 10,
        },
      ],
    },
    {
      metric: "inp",
      days: [
        {
          p75: 351.96,
          count: 8,
        },
        {
          p75: 379.52,
          count: 8,
        },
        {
          p75: 407.08,
          count: 8,
        },
        {
          p75: 420.08,
          count: 8,
        },
        {
          p75: 447.64,
          count: 8,
        },
        {
          p75: 475.2,
          count: 8,
        },
        {
          p75: 433.08,
          count: 8,
        },
      ],
    },
    {
      metric: "cls",
      days: [
        {
          p75: 0.1532,
          count: 8,
        },
        {
          p75: 0.1638,
          count: 8,
        },
        {
          p75: 0.1158,
          count: 8,
        },
        {
          p75: 0.1317,
          count: 8,
        },
        {
          p75: 0.1476,
          count: 8,
        },
        {
          p75: 0.1551,
          count: 8,
        },
        {
          p75: 0.171,
          count: 8,
        },
      ],
    },
    {
      metric: "fcp",
      days: [
        {
          p75: 1517.6,
          count: 6,
        },
        {
          p75: 1602.4,
          count: 6,
        },
        {
          p75: 1687.2,
          count: 6,
        },
        {
          p75: 1772,
          count: 6,
        },
        {
          p75: 1856.8,
          count: 6,
        },
        {
          p75: 1941.6,
          count: 6,
        },
        {
          p75: 2026.4,
          count: 6,
        },
      ],
    },
    {
      metric: "app_ready",
      days: [
        {
          p75: 2220,
          count: 6,
        },
        {
          p75: 2169.6,
          count: 6,
        },
        {
          p75: 2119.2,
          count: 6,
        },
        {
          p75: 2214.6,
          count: 6,
        },
        {
          p75: 2164.2,
          count: 6,
        },
        {
          p75: 2113.8,
          count: 6,
        },
        {
          p75: 2209.2,
          count: 6,
        },
      ],
    },
    {
      metric: "route_ready",
      days: [
        {
          p75: 3012.5,
          count: 8,
        },
        {
          p75: 3176.8,
          count: 8,
        },
        {
          p75: 3341.1,
          count: 8,
        },
        {
          p75: 1486.5,
          count: 8,
        },
        {
          p75: 927,
          count: 8,
        },
        {
          p75: 1027.7,
          count: 8,
        },
        {
          p75: 1116.5,
          count: 8,
        },
      ],
    },
  ],
  slowest_routes: [
    {
      route: "design",
      p75: 3907.5,
      count: 7,
    },
    {
      route: "settings",
      p75: 3450,
      count: 7,
    },
    {
      route: "agents",
      p75: 3176.8,
      count: 7,
    },
    {
      route: "epics",
      p75: 1486.5,
      count: 14,
    },
    {
      route: "stats",
      p75: 1039.8,
      count: 7,
    },
  ],
  build_strip: {
    current: {
      build_id: "b-7c2e55",
      web_version: "1.9.0",
      lcp_p75: 2465,
      inp_p75: 267.72,
      js_p75: 292080,
    },
    previous: {
      build_id: "b-1f3a9c",
      web_version: "1.8.0",
      lcp_p75: 3848,
      inp_p75: 475.2,
      js_p75: 313640,
    },
    lcp_change_pct: -35.9407,
    inp_change_pct: -43.6616,
    js_change_pct: -6.8741,
  },
  why_slow: [
    {
      metric: "ttfb",
      count: 42,
      p75: 222,
      previous_p75: 406.4,
      change_pct: -45.374,
    },
    {
      metric: "api_latency",
      count: 62,
      p75: 291.5,
      previous_p75: 523.5,
      change_pct: -44.3171,
    },
    {
      metric: "js_bytes",
      count: 32,
      p75: 292080,
      previous_p75: 313640,
      change_pct: -6.8741,
    },
    {
      metric: "css_bytes",
      count: 42,
      p75: 32840,
      previous_p75: 36528,
      change_pct: -10.0964,
    },
  ],
  dev_hidden: false,
  poor_inp: {
    count: 44,
    input_delay_p75: 79.04,
    processing_p75: 244.5,
    presentation_delay_p75: 83.8,
    top_actions: [
      {
        action: "sidebar-row",
        count: 13,
      },
      {
        action: "palette",
        count: 12,
      },
      {
        action: "jump-menu",
        count: 10,
      },
    ],
    worst_chunk: "vendor",
  },
}

export const webPerformanceLow: StatsWebPerformance = {
  hard_loads: [
    {
      metric: "lcp",
      has_threshold: true,
      count: 9,
      p75: 3839,
    },
    {
      metric: "inp",
      has_threshold: true,
      count: 6,
      p75: 460.64,
    },
    {
      metric: "cls",
      has_threshold: true,
      count: 6,
      p75: 0.1302,
    },
    {
      metric: "fcp",
      has_threshold: true,
      count: 9,
      p75: 2071.2,
    },
    {
      metric: "app_ready",
      has_threshold: false,
      count: 9,
      p75: 1771.8,
    },
  ],
  route_changes: [
    {
      metric: "route_ready",
      has_threshold: false,
      count: 6,
      p75: 1519.5,
    },
    {
      metric: "inp",
      has_threshold: true,
      count: 6,
      p75: 559.44,
    },
    {
      metric: "cls",
      has_threshold: true,
      count: 6,
      p75: 0.2682,
    },
  ],
  trend: [
    {
      metric: "lcp",
      days: [
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          p75: 3839,
          count: 3,
        },
        {
          p75: 3998,
          count: 3,
        },
        {
          p75: 4157,
          count: 3,
        },
      ],
    },
    {
      metric: "inp",
      days: [
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          p75: 433.08,
          count: 4,
        },
        {
          p75: 460.64,
          count: 4,
        },
        {
          p75: 403.96,
          count: 4,
        },
      ],
    },
    {
      metric: "cls",
      days: [
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          p75: 0.1308,
          count: 4,
        },
        {
          p75: 0.1467,
          count: 4,
        },
        {
          p75: 0.1626,
          count: 4,
        },
      ],
    },
    {
      metric: "fcp",
      days: [
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          p75: 2071.2,
          count: 3,
        },
        {
          p75: 2156,
          count: 3,
        },
        {
          p75: 2240.8,
          count: 3,
        },
      ],
    },
    {
      metric: "app_ready",
      days: [
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          p75: 1771.8,
          count: 3,
        },
        {
          p75: 1867.2,
          count: 3,
        },
        {
          p75: 1962.6,
          count: 3,
        },
      ],
    },
    {
      metric: "route_ready",
      days: [
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          count: 0,
        },
        {
          p75: 1440,
          count: 2,
        },
        {
          p75: 1519.5,
          count: 2,
        },
        {
          p75: 1599,
          count: 2,
        },
      ],
    },
  ],
  slowest_routes: [
    {
      route: "epics",
      p75: 1519.5,
      count: 6,
    },
  ],
  build_strip: {
    current: {
      build_id: "b-0a1b2c",
      web_version: "1.0.0",
      lcp_p75: 3839,
      inp_p75: 460.64,
      js_p75: 314640,
    },
  },
  why_slow: [
    {
      metric: "ttfb",
      count: 9,
      p75: 405.2,
    },
    {
      metric: "api_latency",
      count: 15,
      p75: 478.5,
    },
    {
      metric: "js_bytes",
      count: 7,
      p75: 314640,
    },
    {
      metric: "css_bytes",
      count: 9,
      p75: 36504,
    },
  ],
  dev_hidden: false,
  poor_inp: {
    count: 8,
    input_delay_p75: 82.8,
    processing_p75: 256.25,
    presentation_delay_p75: 88.5,
    top_actions: [
      {
        action: "palette",
        count: 3,
      },
      {
        action: "sidebar-row",
        count: 2,
      },
      {
        action: "jump-menu",
        count: 2,
      },
    ],
    worst_chunk: "vendor",
  },
}
