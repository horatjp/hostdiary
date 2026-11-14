/* Pure chart definitions and transformations. Values use display units (net: bit/s). */
(function () {
  'use strict';
  const finite = value => typeof value === 'number' && Number.isFinite(value);
  const bootTime = value => Number.isInteger(value) && Number.isFinite(new Date(value * 1000).getTime());
  // Keep recorded order for the header; sorting is only for visible markers.
  function latestBoot(boots) {
    return Array.isArray(boots) && bootTime(boots.at(-1)) ? boots.at(-1) : null;
  }
  function selectBoots(boots, from, to) {
    if (!finite(from) || !finite(to) || from > to) throw new RangeError('Invalid boot selection window.');
    return [...new Set((Array.isArray(boots) ? boots : []).filter(value => bootTime(value) && value >= from && value <= to))].sort((a, b) => a - b);
  }
  function stackSeries(columns) {
    if (!Array.isArray(columns) || columns.some(column => !Array.isArray(column) || column.length !== columns[0].length)) throw new TypeError('Stack columns must have equal lengths.');
    const result = columns.map(column => column.map(() => null));
    for (let i = 0; i < (columns[0]?.length || 0); i++) {
      if (columns.some(column => !finite(column[i]))) continue;
      let sum = 0;
      columns.forEach((column, j) => { sum += column[i]; result[j][i] = sum; });
    }
    return result;
  }
  function mirrorSeries(values) { return values.map(value => finite(value) ? -value : null); }
  function mirrorRange(min, max) {
    const m = Math.max(finite(min) ? Math.abs(min) : 0, finite(max) ? Math.abs(max) : 0) || 1;
    return [-m, m];
  }
  // Area heights represent quantities from zero, including a cumulative stack.
  function zeroRange(max) { return [0, finite(max) && max > 0 ? max * 1.05 : 1]; }
  // samples weights interval means; max supplies one-minute peaks. Omit max for
  // derived ratios. seconds integrates display rates (bit/s -> bit for net).
  function seriesStats(values, options = {}) {
    if (!Array.isArray(values) || ['samples','max'].some(key => options[key] != null && (!Array.isArray(options[key]) || options[key].length !== values.length))) throw new TypeError('Statistics arrays must have equal lengths.');
    let min = null, max = null, last = null, sum = 0, weight = 0;
    values.forEach((value, i) => {
      const samples = options.samples ? options.samples[i] : 1;
      if (!finite(value) || !finite(samples) || samples <= 0) return;
      min = min === null ? value : Math.min(min, value);
      last = value;
      const peak = options.max ? options.max[i] : value;
      if (finite(peak)) max = max === null ? peak : Math.max(max, peak);
      sum += value * samples; weight += samples;
    });
    return {min, avg:weight > 0 ? sum / weight : null, max, last,
      total:weight > 0 && finite(options.seconds) && options.seconds > 0 ? sum * options.seconds : null};
  }
  // The first point has no preceding boundary; it does not define a visible bar.
  function cursorIndex(times, value) {
    if (!finite(value) || times.length < 2 || value <= times[0] || value > times.at(-1)) return null;
    let lo = 1, hi = times.length - 1;
    while (lo < hi) {
      const mid = Math.floor((lo + hi) / 2);
      if (times[mid] < value) lo = mid + 1; else hi = mid;
    }
    return lo;
  }
  function plotTime(time, step, coarse) { return coarse ? time + step : time; }
  function diskUsage(used, avail, inodesUsed, inodesTotal) {
    return {used:finite(used) && finite(avail) ? ratioValue(used, used + avail, 100) : null,
      inodes:ratioValue(inodesUsed, inodesTotal, 100)};
  }
  function swapFree(total, used) {
    return finite(total) && finite(used) && total >= used ? total - used : null;
  }

  // Fixed per-kind palettes: removing a column never changes any remaining color.
  const palettes = {
    cpu:['#245ac6','#60a5fa','#cc2222','#e6ae22','#8945aa','#dc73a8','#697493'],
    load:['#008800','#245ac6','#cc2222','#697493'], mem:['#245ac6','#008800','#8945aa'],
    swap:['#245ac6','#008800','#008800','#cc2222'],
    procs:['#008800','#245ac6','#e6ae22','#8945aa','#cc2222','#697493'],
    tcp:['#008800','#245ac6','#60a5fa','#e6ae22','#9a5713','#8945aa','#cc2222','#dc73a8','#087d87','#557425','#9c582f','#697493','#008800','#cc2222','#8945aa'],
    net:['#008800','#cc2222','#245ac6','#087d87','#8945aa','#9a5713'],
    disk:['#245ac6','#008800','#245ac6','#697493'],
    diskio:['#008800','#cc2222','#008800','#cc2222','#245ac6','#008800','#cc2222'],
    psi:['#008800','#245ac6','#8945aa','#e6ae22','#cc2222']
  };
  const fallbackColors = ['#245ac6','#008800','#cc2222','#8945aa','#e6ae22','#087d87'];
  const psiColumns = ['cpu_some','memory_some','memory_full','io_some','io_full'];
  function panelsFor(group) {
    if (!Array.isArray(group.columns) || !Array.isArray(group.units) || group.columns[0] !== 'time' || group.columns.length !== group.units.length) throw new TypeError('Panel columns and units must align, starting with time.');
    const expectedUnit = column => {
      if (!Object.hasOwn(palettes, group.kind)) return group.units[group.columns.indexOf(column)];
      if (column === 'swap_free') return 'bytes';
      if (column === 'retrans_ratio') return '%';
      if (['read_completion','write_completion'].includes(column)) return 'ms';
      switch (group.kind) {
        case 'cpu': case 'psi': return '%';
        case 'load': return column === 'cpus' ? 'count' : '(none)';
        case 'mem': return column === 'oom_kills' ? 'count' : 'bytes';
        case 'swap': return ['in','out'].includes(column) ? 'bytes/s' : 'bytes';
        case 'procs': return 'count';
        case 'tcp': return ['out_segs','retrans_segs'].includes(column) ? 'count/s' : 'count';
        case 'net': return ['rx','tx'].includes(column) ? 'bytes/s' : 'count/s';
        case 'disk': return column.startsWith('inodes_') ? 'count' : 'bytes';
        case 'diskio': return ['read','write'].includes(column) ? 'bytes/s' : ['read_ops','write_ops'].includes(column) ? 'count/s' : column === 'busy' ? '%' : 'ms/s';
        default: return group.units[group.columns.indexOf(column)];
      }
    };
    const has = column => { const i = group.columns.indexOf(column); return i > 0 && group.units[i] === expectedUnit(column); };
    const derived = {swap_free:['total','used'], retrans_ratio:['retrans_segs','out_segs'],
      read_completion:['read_time','read_ops'], write_completion:['write_time','write_ops']};
    const available = column => has(column) || (Object.hasOwn(derived, column) && derived[column].every(has));
    const panels = [];
    // Define colors before filtering columns, preserving identity across old headers.
    const add = (title, unit, mode, columns, offset = 0, extra = {}) => {
      const palette = Object.hasOwn(palettes, group.kind) ? palettes[group.kind] : fallbackColors;
      const series = columns.map((column, i) => ({column,
        label:'column.' + (group.kind === 'tcp' && column === 'other' ? 'tcp_other' : column),
        color:palette[(group.kind === 'psi' && psiColumns.includes(column) ? psiColumns.indexOf(column) : i + offset) % palette.length]})).filter(item => available(item.column));
      if (!series.length) return;
      panels.push({title:'panel.' + title, unit, mode, series, tableOnly:[], notes:[], ...extra});
    };
    switch (group.kind) {
      case 'cpu':
        add('cpu','%','stack',['user','nice','system','iowait','irq','softirq','steal'],0,{range:[0,100],tableOnly:['idle'].filter(has)}); break;
      case 'load':
        add('load','(none)','line',['load1','load5','load15','cpus']);
        panels[0]?.series.forEach(series => { if (series.column === 'cpus') { series.dash = [6,4]; series.fill = false; } }); break;
      case 'mem':
        add('mem','bytes','stack',['used','available'],0,{tableOnly:['buffers','cached','total'].filter(has)});
        add('oom','count','line',['oom_kills'],2); break;
      case 'swap':
        add('swap','bytes','stack',['used','swap_free']);
        add('swapio','bytes/s','mirror',['in','out'],2); break;
      case 'procs':
        add('procs','count','stack',['running','sleeping','disk_sleep','stopped','zombie','other'],0,{tableOnly:['total'].filter(has)}); break;
      case 'tcp':
        add('tcp','count','stack',['established','syn_sent','syn_recv','fin_wait1','fin_wait2','time_wait','close','close_wait','last_ack','listen','closing','other']);
        add('segments','count/s','line',['out_segs','retrans_segs'],12);
        add('retrans','%','line',['retrans_ratio'],14,{notes:['note.ratio']}); break;
      case 'net':
        add('net','bit/s','mirror',['rx','tx'],0,{totalUnit:'bit',notes:['note.total']});
        add('errors','count/s','line',['rx_errors','rx_drops','tx_errors','tx_drops'],2);
        panels.find(panel => panel.title === 'panel.errors')?.series.forEach(series => { series.fill = false; }); break;
      case 'disk':
        add('disk','bytes','stack',['used','avail'],0,{notes:['note.disk']});
        add('inodes','count','line',['inodes_used','inodes_total'],2);
        panels.find(panel => panel.title === 'panel.inodes')?.series.forEach(series => { if (series.column === 'inodes_total') series.fill = false; }); break;
      case 'diskio':
        add('diskio','bytes/s','mirror',['read','write']);
        add('operations','count/s','mirror',['read_ops','write_ops'],2);
        add('busy','%','line',['busy'],4);
        add('completion','ms','line',['read_completion','write_completion'],5,{notes:['note.ratio']}); break;
      case 'psi':
        add('psi','%','line',group.columns.slice(1).filter(column => !column.endsWith('_max') && !['samples','last_time'].includes(column))); break;
      default: {
        const byUnit = new Map();
        group.columns.slice(1).forEach((column, i) => {
          if (column.endsWith('_max') || ['samples','last_time'].includes(column)) return;
          const unit = group.units[i + 1];
          if (!byUnit.has(unit)) byUnit.set(unit, []);
          byUnit.get(unit).push(column);
        });
        for (const [unit, columns] of byUnit) add('generic',unit,'line',columns);
      }
    }
    return panels;
  }

  function chartGroup(group, resolution) {
    if (resolution === '1m') return group;
    const columns = ['time'], units = ['s'];
    group.columns.slice(1).forEach((column, i) => {
      if (column === 'samples' || column === 'last_time' || column.endsWith('_max')) return;
      columns.push(column, column + '_max'); units.push(group.units[i + 1], group.units[i + 1]);
    });
    return {...group, columns, units};
  }
  // Same-row materials only. Coarse maxima detect rounding, never supply a ratio.
  function ratioValue(numerator, denominator, factor, numeratorMax, denominatorMax, coarse = false) {
    if (typeof numerator !== 'number' || !Number.isFinite(numerator) || typeof denominator !== 'number' || !Number.isFinite(denominator) || denominator <= 0) return null;
    if (coarse && ((numerator === 0 && numeratorMax > 0) || (denominator === 0 && denominatorMax > 0))) return null;
    const value = numerator / denominator * factor;
    return Number.isFinite(value) ? value : null;
  }
  // Raw arrays feed the table; plotSeries is the single index mapping for uPlot.
  // Stack painting reverses the cumulative arrays, never the table's column order.
  function prepareCharts(group, data, resolution) {
    const base = chartGroup(group, resolution);
    const definitions = panelsFor(group);
    const candidates = group.kind === 'diskio' ? [
      {column:'read_completion', unit:'ms', numerator:'read_time', denominator:'read_ops', factor:1},
      {column:'write_completion', unit:'ms', numerator:'write_time', denominator:'write_ops', factor:1}
    ] : group.kind === 'tcp' ? [
      {column:'retrans_ratio', unit:'%', numerator:'retrans_segs', denominator:'out_segs', factor:100}
    ] : group.kind === 'swap' ? [{column:'swap_free', unit:'bytes'}] : [];
    const derived = candidates.filter(d => definitions.some(p => p.series.some(s => s.column === d.column)));
    const columns = [...base.columns], units = [...base.units], values = [...data];
    const material = column => values[columns.indexOf(column)];
    for (const d of derived) {
      const result = data[0].map((_, i) => d.column === 'swap_free' ? swapFree(material('total')?.[i], material('used')?.[i]) :
        ratioValue(material(d.numerator)?.[i], material(d.denominator)?.[i], d.factor, material(d.numerator + '_max')?.[i], material(d.denominator + '_max')?.[i], resolution !== '1m'));
      values.push(result); columns.push(d.column); units.push(d.unit);
    }
    const plotted = {...base, columns, units};
    const panels = definitions.filter(panel => !panel.series.every(series => derived.some(d => d.column === series.column)) ||
      panel.series.some(series => material(series.column)?.some(finite))).map(panel => {
      const raw = Object.fromEntries([...panel.series.map(s => s.column), ...panel.tableOnly].map(column => [column, material(column) || data[0].map(() => null)]));
      const original = panel.series.map(series => raw[series.column]);
      let transformed = original;
      let plotSeries = panel.series.map(series => ({...series}));
      if (panel.mode === 'stack') { transformed = stackSeries(original).reverse(); plotSeries.reverse(); }
      if (panel.mode === 'mirror') transformed = panel.series.map((series, i) => ['tx','out','write','write_ops'].includes(series.column) ? mirrorSeries(original[i]) : original[i]);
      if (panel.mode === 'line' && resolution !== '1m') {
        for (const series of panel.series) {
          const max = material(series.column + '_max');
          if (max) { plotSeries.push({...series, maxLine:true}); transformed.push(max); }
        }
      }
      return {...panel, raw, plotSeries, data:[data[0], ...transformed],
        group:{...group, columns:['time', ...plotSeries.map(s => s.column + (s.maxLine ? '_max' : ''))], units:['s', ...plotSeries.map(() => panel.unit)]}};
    });
    return {plotted, data:values, panels, derived};
  }
  const api = {selectBoots, latestBoot, panelsFor, stackSeries, mirrorSeries, mirrorRange, zeroRange, seriesStats, plotTime, cursorIndex, diskUsage, swapFree, chartGroup, ratioValue, prepareCharts};
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else window.HDViews = api;
}());
