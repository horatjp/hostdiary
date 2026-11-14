/* Offline script loader and pure schema-1 chart preparation. No fetch or imports. */
(function () {
  'use strict';

  const commonJS = typeof module !== 'undefined' && module.exports;
  const {detectLanguage, messages, t} = commonJS ? require('./i18n.js') : window.HDI18n;
  const {selectBoots, latestBoot, chartGroup, prepareCharts, seriesStats, cursorIndex, diskUsage, plotTime, mirrorRange, zeroRange} = commonJS ? require('./views.js') : window.HDViews;
  const lang = commonJS ? 'en' : detectLanguage(navigator.languages || [navigator.language]);
  const locale = language => language === 'ja' ? 'ja-JP' : 'en-US';
  const tr = (key, params) => t(lang, key, params);
  const coarseNotice = tr('note.coarse');
  const columnLabel = (kind, column) => {
    const key = 'column.' + (kind === 'tcp' && column === 'other' ? 'tcp_other' : column);
    return Object.hasOwn(messages.en, key) ? tr(key) : column;
  };
  function periodPlan(seconds) {
    return {resolution: seconds === 2678400 ? '5m' : seconds === 31536000 ? '1h' : seconds === 63072000 ? '1d' : '1m'};
  }
  function historyKeys(group, from, to, resolution) {
    if (resolution === '1m') return neededDays(group.days || [], from, to).map(day => '1m/' + day + '/' + group.id);
    return [...new Set(group.coarse?.[resolution] || [])].filter(period => {
      const year = resolution === '1d';
      if (typeof period !== 'string' || !(year ? /^\d{4}$/ : /^\d{4}-\d{2}$/).test(period)) return false;
      const date = new Date(period + (year ? '-01-01' : '-01') + 'T00:00:00Z');
      if (!Number.isFinite(+date) || date.toISOString().slice(0, year ? 4 : 7) !== period) return false;
      const start = +date / 1000;
      if (year) date.setUTCFullYear(date.getUTCFullYear() + 1); else date.setUTCMonth(date.getUTCMonth() + 1);
      return start <= to && +date / 1000 > from;
    }).sort().map(period => resolution + '/' + period + '/' + group.id);
  }
  function formatTime(timestamp, utcDay) {
    return utcDay ? new Date(timestamp * 1000).toISOString().slice(0, 10) + ' (' + tr('chart.utcDay') + ')' : new Intl.DateTimeFormat(locale(lang), {year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'}).format(new Date(timestamp * 1000));
  }
  function dateForPlot(timestamp, utcDay) {
    const date = new Date(timestamp * 1000);
    // uPlot formats local Date fields; for daily charts make those fields UTC.
    return utcDay ? new Date(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate(), date.getUTCHours(), date.getUTCMinutes(), date.getUTCSeconds()) : date;
  }
  function trafficLines(traffic) {
    if (!traffic) return [];
    return [['traffic.month', traffic.month], ['traffic.last30d', traffic.last30d]].map(([label, value]) => {
      const amount = value.days_missing >= value.days ? tr('traffic.empty') : columnLabel('net','rx') + ' ' + formatValue(value.rx, 'bytes') + ' / ' + columnLabel('net','tx') + ' ' + formatValue(value.tx, 'bytes');
      return tr(label) + ': ' + amount + (value.days_missing ? ' · ' + tr(value.days_missing === 1 ? 'traffic.missingOne' : 'traffic.missing',{count:value.days_missing}) : '');
    });
  }

  function displayUnit(kind, unit) {
    return kind === 'net' && unit === 'bytes/s' ? 'bit/s' : unit;
  }
  function displayValue(kind, unit, value) {
    if (typeof value !== 'number' || !Number.isFinite(value)) return null;
    return kind === 'net' && unit === 'bytes/s' ? value * 8 : value;
  }
  function seriesLabel(column, unit) {
    return ['(none)','count','count/s'].includes(unit) ? column : column + ' (' + unit + ')';
  }
  // Binary increments keep byte axes aligned with B/KiB/MiB boundaries.
  // Include fractional bytes for low rates and powers through uint64's range.
  function axisIncrements(unit) {
    if (unit !== 'bytes' && unit !== 'bytes/s') return undefined;
    return Array.from({length:75}, (_, i) => 2 ** (i - 10));
  }
  // Observations are nonnegative. One formatter supplies axes, legends and cells;
  // missing/invalid values never acquire a unit, and -0 is displayed as 0.
  function formatValue(value, unit) {
    if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return '—';
    let scaled = value === 0 ? 0 : value, suffix = unit;
    const binary = unit === 'bytes' || unit === 'bytes/s';
    const labels = binary ? ['B','KiB','MiB','GiB','TiB'] : unit === 'bit' ? ['bit','kbit','Mbit','Gbit','Tbit','Pbit'] : ['bit/s','kbit/s','Mbit/s','Gbit/s'];
    if (binary || unit === 'bit/s' || unit === 'bit') {
      const base = binary ? 1024 : 1000;
      let i = 0;
      while (Math.round(scaled * 1000) / 1000 >= base && i < labels.length - 1) { scaled /= base; i++; }
      suffix = labels[i] + (unit === 'bytes/s' ? '/s' : '');
    }
    const text = new Intl.NumberFormat(locale(lang), {maximumFractionDigits:3}).format(scaled);
    return !suffix || ['(none)','count','count/s'].includes(suffix) ? text : text + ' ' + suffix;
  }
  function loadError(source, path) {
    return tr('error.' + (['index','uplot','data'].includes(source) ? source : 'display'),{path});
  }
  function neededDays(days, from, to) {
    return [...new Set(days)].filter(day => {
      if (typeof day !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(day)) return false;
      const start = Date.parse(day + 'T00:00:00Z') / 1000;
      return Number.isFinite(start) && new Date(start * 1000).toISOString().slice(0, 10) === day && start <= to && start + 86400 > from;
    }).sort();
  }

  // Each file starts without a header. Later headers change only subsequent rows.
  // Output columns follow current index metadata; missing old columns remain null.
  function buildSeries(files, group, from, to, maxX = Infinity) {
    const coarse = group.columns.some(column => column.endsWith('_max'));
    const points = new Map();
    for (const lines of files) {
      let header = null;
      for (const line of lines) {
        if (line && !Array.isArray(line) && typeof line === 'object') {
          header = line.schema === 1 && line.group === group.id && Array.isArray(line.columns) && Array.isArray(line.units) && line.columns[0] === 'time' && line.columns.length === line.units.length && Number.isFinite(line.step) && line.step > 0 ? line : null;
          continue;
        }
        if (!header || !Array.isArray(line) || line.length !== header.columns.length || !Number.isFinite(line[0]) || line[0] < from || line[0] > to) continue;
        if (line.some(value => value !== null && (typeof value !== 'number' || !Number.isFinite(value)))) continue;
        const x = plotTime(line[0], header.step, coarse);
        if (x > maxX) continue;
        const samples = line[header.columns.indexOf('samples')];
        const validSamples = Number.isFinite(samples) && samples > 0;
        const values = group.columns.slice(1).map((column, j) => {
          const i = header.columns.indexOf(column);
          return (!coarse || validSamples) && i > 0 && header.units[i] === group.units[j + 1] ? displayValue(group.kind, header.units[i], line[i]) : null;
        });
        points.set(x, {time:line[0], x, step:header.step, values, samples:validSamples ? samples : null});
      }
    }
    const data = group.columns.map(() => []), samples = coarse ? [] : null, times = [];
    let previous = null;
    for (const point of [...points.values()].sort((a, b) => a.x - b.x)) {
      if (previous && point.x - previous.x > previous.step * 1.5) {
        data[0].push((previous.x + point.x) / 2); times.push(null); samples?.push(null);
        for (const column of data.slice(1)) column.push(null);
      }
      data[0].push(point.x); times.push(point.time); samples?.push(point.samples);
      point.values.forEach((value, i) => data[i + 1].push(value));
      previous = point;
    }
    // Original point times keep labels faithful even if a retained header has a
    // different step. Axis ticks remain interval boundaries, without subtraction.
    return {data, samples, times};
  }

  // Daily directory rows compare identical mount/path pairs only; absence is not zero.
  function directoryDays(days) {
    const valid = neededDays(days || [], -Infinity, Infinity);
    if (!valid.length) return [];
    if (valid.length === 1) return valid;
    const latest = valid.at(-1), week = new Date(Date.parse(latest + 'T00:00:00Z') - 7 * 86400000).toISOString().slice(0, 10);
    return [latest, valid.includes(week) ? week : valid[0]];
  }
  function directoryStatus(fs) {
    if (fs.complete) return tr('dirs.complete');
    return Number.isFinite(fs.errors) ? tr('dirs.skipped',{count:fs.errors}) : tr('dirs.partial');
  }
  function directoryTables(latest, previous) {
    return (latest.filesystems || []).map(fs => {
      const old = previous?.filesystems?.find(item => item.mount === fs.mount);
      const values = new Map((old?.entries || []).map(entry => [entry.path, entry.bytes]));
      return {...fs, status: directoryStatus(fs), rows: (fs.entries || []).map(entry => {
        const before = values.get(entry.path);
        const valid = typeof before === 'number' && Number.isFinite(before) && before >= 0;
        return {...entry, previous: valid ? before : null, difference: valid ? entry.bytes - before : null};
      })};
    });
  }
  function formatDifference(value) {
    if (typeof value !== 'number' || !Number.isFinite(value)) return '—';
    return (value < 0 ? '-' : value > 0 ? '+' : '') + formatValue(Math.abs(value), 'bytes');
  }

  // Event times are independent of observations and never affect data or scales.
  function bootMarkers(boots, onError = error => { throw error; }) {
    return {hooks:{draw:[u => {
      const {min, max} = u.scales.x;
      if (!Number.isFinite(min) || !Number.isFinite(max) || min > max) return;
      const visible = selectBoots(boots, min, max);
      if (!visible.length) return;
      const ctx = u.ctx, box = u.bbox, ratio = ctx.canvas.width / u.width;
      ctx.save();
      try {
        ctx.beginPath(); ctx.rect(box.left, box.top, box.width, box.height); ctx.clip();
        ctx.strokeStyle = '#53677f'; ctx.lineWidth = ratio; ctx.setLineDash([3 * ratio, 3 * ratio]);
        ctx.beginPath();
        for (const boot of visible) {
          const x = u.valToPos(boot, 'x', true);
          ctx.moveTo(x, box.top); ctx.lineTo(x, box.top + box.height);
        }
        ctx.stroke();
      } catch (error) { onError(error); }
      finally { ctx.restore(); }
    }]}};
  }

  if (typeof module !== 'undefined' && module.exports) {
    module.exports = {bootMarkers, directoryDays, directoryTables, formatDifference, buildSeries, neededDays, displayUnit, displayValue, formatValue, loadError, seriesLabel, axisIncrements, periodPlan, historyKeys, formatTime, trafficLines, coarseNotice};
    return;
  }

  const records = new Map(), loads = new Map(), directoryRecords = new Map();
  let index = null, generation = 0, charts = [], observers = [];
  window.HD = {
    index(value) { index = value; },
    dirs(day, value) { directoryRecords.set(day, value); },
    l(path, line) {
      if (!records.has(path)) records.set(path, []);
      records.get(path).push(line);
    }
  };
  const el = id => document.getElementById(id);
  const node = (tag, text, className) => {
    const element = document.createElement(tag);
    if (text !== undefined) element.textContent = text;
    if (className) element.className = className;
    return element;
  };
  const localTime = timestamp => formatTime(timestamp, false);
  function loadScript(src) {
    return new Promise((resolve, reject) => {
      const script = document.createElement('script');
      script.src = src;
      script.onload = () => { script.remove(); resolve(); };
      script.onerror = () => { script.remove(); reject(new Error(tr('error.load',{path:src}))); };
      document.head.append(script);
    });
  }
  function loadDay(key) {
    if (!loads.has(key)) {
      const promise = loadScript('data/' + key + '.js').then(() => {
        if (!records.has(key)) throw new Error(tr('error.readable',{path:'data/' + key + '.js'}));
      }).catch(() => { loads.delete(key); records.delete(key); throw new Error(loadError('data', 'data/' + key + '.js')); });
      loads.set(key, promise);
    }
    return loads.get(key);
  }
  function reportError(message) {
    el('load-errors').hidden = false;
    el('load-errors').append(node('li', message));
  }
  function showUnavailable() {
    const list = el('unavailable-list');
    list.replaceChildren();
    for (const item of index.unavailable || []) {
      const kind = index.groups.find(group => group.id === item.group)?.kind || item.group;
      const name = Object.hasOwn(messages.en,'kind.' + item.group) ? tr('kind.' + item.group) : item.group;
      const target = name + ' / ' + (item.column ? columnLabel(kind,item.column) : tr('allColumns'));
      const reason = Object.hasOwn(messages.en,'reason.' + item.reason) ? tr('reason.' + item.reason) : item.reason;
      list.append(node('li', target + ': ' + reason));
    }
    el('unavailable').hidden = list.children.length === 0;
  }
  async function showDirectories(token) {
    const container = el('directories');
    container.replaceChildren();
    const days = directoryDays(index.dirs?.days);
    container.hidden = days.length === 0;
    if (!days.length) return;
    try {
      await Promise.all([...new Set(days)].map(async day => {
        await loadScript('data/dirs/' + day + '.js');
        const doc = directoryRecords.get(day);
        if (!doc || doc.schema !== 1 || doc.kind !== 'dirs' || !Array.isArray(doc.filesystems)) throw new Error(tr('error.directorySchema',{day}));
      }));
      if (token !== generation) return;
      const section = node('section', undefined, 'group directory-usage');
      const heading = node('div', undefined, 'group-heading');
      heading.append(node('h2', tr('dirs.title'))); section.append(heading);
      section.append(node('p', tr('dirs.day',{day:days[0]}) + (days[1] ? ' ' + tr('dirs.compare',{day:days[1]}) : '') + '. ' + tr('dirs.note'), 'traffic'));
      if (!days[1]) section.append(node('p', tr('dirs.noEarlier'), 'traffic'));
      section.append(node('p', tr('dirs.absent'), 'traffic'));
      const previous = directoryRecords.get(days[1]);
      for (const fs of directoryTables(directoryRecords.get(days[0]), previous)) {
        const details = node('div', undefined, 'directory-table');
        const table = node('table');
        const old = previous?.filesystems.find(item => item.mount === fs.mount);
        table.append(node('caption', fs.mount + ' · ' + tr('dirs.total') + ' ' + formatValue(fs.total, 'bytes') + ' · ' + fs.status + (old && !old.complete ? ' · ' + tr('dirs.comparison') + ' ' + directoryStatus(old) : '')));
        const thead = node('thead'), head = node('tr');
        for (const label of [tr('dirs.path'), tr('dirs.depth'), days[0], days[1] || '—', tr('dirs.change')]) { const th = node('th', label); th.scope = 'col'; head.append(th); }
        thead.append(head); table.append(thead);
        const body = node('tbody');
        for (const row of fs.rows) {
          const tr = node('tr'), path = node('th', row.path); path.scope = 'row'; tr.append(path);
          for (const text of [String(row.depth), formatValue(row.bytes, 'bytes'), formatValue(row.previous, 'bytes'), formatDifference(row.difference)]) tr.append(node('td', text, 'latest'));
          body.append(tr);
        }
        table.append(body); details.append(table); section.append(details);
      }
      container.append(section);
    } catch (error) { if (token === generation) reportError(tr('error.dirs',{message:error.message})); }
  }
  // uPlot 1.6.32's default time table, with numeric months and 24-hour times.
  const timeAxis = [
    [31536000,'{YYYY}',null,null,null,null,null,null,1],
    [2419200,'{M}/{D}','\n{YYYY}',null,null,null,null,null,1],
    [86400,'{M}/{D}','\n{YYYY}',null,null,null,null,null,1],
    [3600,'{HH}:{mm}','\n{M}/{D}/{YY}',null,'\n{M}/{D}',null,null,null,1],
    [60,'{HH}:{mm}','\n{M}/{D}/{YY}',null,'\n{M}/{D}',null,null,null,1],
    [1,':{ss}','\n{M}/{D}/{YY} {HH}:{mm}',null,'\n{M}/{D} {HH}:{mm}',null,'\n{HH}:{mm}',null,1],
    [0.001,':{ss}.{fff}','\n{M}/{D}/{YY} {HH}:{mm}',null,'\n{M}/{D} {HH}:{mm}',null,'\n{HH}:{mm}',null,1]
  ];
  function statsTable(section, panel, prepared, history, seconds, from, to, utcDay) {
    const wrapper = node('div', undefined, 'stats-wrap'), table = node('table', undefined, 'stats');
    const caption = node('caption', tr('stats.period',{period:tr('period.' + seconds)}) + ' · ' + formatTime(from, false) + ' – ' + formatTime(to, false));
    table.append(caption);
    const head = node('tr');
    for (const key of ['series','min','avg','max','last','cursor',...(panel.totalUnit ? ['total'] : [])]) { const cell = node('th', tr('stats.' + key)); cell.scope = 'col'; head.append(cell); }
    const thead = node('thead'); thead.append(head); table.append(thead);
    const body = node('tbody'), cursorCells = [];
    const specs = [...panel.series, ...panel.tableOnly.map(column => ({column}))];
    for (const spec of specs) {
      const values = panel.raw[spec.column];
      const max = prepared.data[prepared.plotted.columns.indexOf(spec.column + '_max')];
      const stats = seriesStats(values, {samples:history.samples, max, seconds:panel.totalUnit ? 60 : undefined});
      const row = node('tr', undefined, 'stats-row'); row.dataset.column = spec.column;
      const label = node('th', undefined); label.scope = 'row';
      if (spec.color) { const swatch = node('span', undefined, 'swatch'); swatch.style.backgroundColor = spec.color; label.append(swatch); }
      label.append(node('span', columnLabel(prepared.plotted.kind, spec.column))); row.append(label);
      for (const key of ['min','avg','max','last']) row.append(node('td', formatValue(stats[key], panel.unit), 'stat-value'));
      const cursor = node('td', '—', 'stat-value'); row.append(cursor); cursorCells.push({cell:cursor, values});
      if (panel.totalUnit) row.append(node('td', formatValue(stats.total, panel.totalUnit), 'stat-value'));
      body.append(row);
    }
    table.append(body); wrapper.append(table); section.append(wrapper);
    const time = node('p', tr('stats.cursor') + ': —', 'chart-note'); section.append(time);
    return u => {
      const i = u.cursor.left == null || u.cursor.left < 0 ? null : cursorIndex(history.data[0], u.posToVal(u.cursor.left,'x'));
      const valid = i !== null && history.times[i] != null && (history.samples === null || history.samples[i] > 0);
      for (const item of cursorCells) item.cell.textContent = formatValue(valid ? item.values[i] : null, panel.unit);
      time.textContent = tr('stats.cursor') + ': ' + (valid ? formatTime(history.times[i], utcDay) : '—');
    };
  }
  function groupTitle(group) {
    const title = Object.hasOwn(messages.en,'kind.' + group.kind) ? tr('kind.' + group.kind) : group.kind;
    return title + (group.name !== group.kind ? ' · ' + group.name : '');
  }
  function showContents(groups) {
    const list = el('contents-list'); list.replaceChildren();
    for (const group of groups) {
      const item = node('li'), link = node('a', groupTitle(group));
      link.href = '#group-' + group.id;
      item.append(link); list.append(item);
    }
    el('contents').hidden = groups.length === 0;
  }
  function draw(group, history, from, to, resolution, seconds) {
    const prepared = prepareCharts(group, history.data, resolution), utcDay = resolution === '1d';
    const selectedBoots = selectBoots(index.boots, from, to);
    const section = node('section', undefined, 'group');
    section.id = 'group-' + group.id;
    const heading = node('div', undefined, 'group-heading');
    heading.append(node('h2', groupTitle(group)));
    if (group.last_seen < index.generated_at) heading.append(node('span', tr('status.stopped',{time:localTime(group.last_seen)}), 'stopped'));
    section.append(heading); el('groups').append(section);
    if (!prepared.panels.length) section.append(node('p',tr('chart.empty'),'chart-note'));
    for (const panel of prepared.panels) {
      section.append(node('h3', tr(panel.title)));
      const plot = node('div', undefined, 'chart'); section.append(plot);
      const updateCursor = statsTable(section, panel, prepared, history, seconds, from, to, utcDay);
      const values = panel.data.slice(1);
      const observed = values.some(column => column.some(value => typeof value === 'number' && Number.isFinite(value)));
      if (!observed) plot.append(node('p', tr('chart.empty')));
      if (observed || selectedBoots.length > 0) {
        // Initial bounds use the selected period; explicit drag bounds must pass
        // through instead of being reset by a constant range callback.
        const scales = {x:{time:true,min:from,max:to,range:(_u,min,max) => [min ?? from,max ?? to]}, y:{time:false,range:(_u,_min,max) => zeroRange(max)}};
        if (panel.range) scales.y.range = () => panel.range;
        if (panel.mode === 'mirror') scales.y.range = (_u, min, max) => mirrorRange(min, max);
        const axes = [{font:'11px monospace',stroke:'#53677f',grid:{stroke:'#e3eaf1'},values:utcDay ? (_u,ticks) => ticks.map(time => new Date(time * 1000).toISOString().slice(0,10)) : timeAxis},
          {scale:'y',label:['count','count/s'].includes(panel.unit) ? tr('unit.' + panel.unit) : panel.unit === '(none)' ? '' : panel.unit,
            incrs:axisIncrements(panel.unit),size:82,font:'11px monospace',stroke:'#53677f',grid:{stroke:'#e3eaf1'},
            values:(_u,ticks) => ticks.map(value => formatValue(panel.mode === 'mirror' ? Math.abs(value) : value,panel.unit))}];
        const paths = uPlot.paths.stepped({align:-1});
        const series = [{label:tr('chart.time')}, ...panel.plotSeries.map(spec => ({label:columnLabel(group.kind,spec.column),scale:'y',
          stroke:spec.color + (spec.maxLine ? '77' : ''), width:spec.maxLine ? 1 : 1.5, dash:spec.dash, paths,
          fill:spec.maxLine || spec.fill === false ? null : spec.color + (panel.mode === 'stack' ? '' : '33'),
          spanGaps:false,points:{show:false}}))];
        let chart, observer, failed = false;
        const note = node('p',undefined,'boot-note');
        const fallback = error => {
          if (observed) throw error;
          if (failed) return;
          failed = true; console.error('Empty history chart unavailable:', error);
          // draw may run inside construction. Defer disposal until uPlot returns.
          Promise.resolve().then(() => {
            observer?.disconnect(); chart?.destroy();
            charts = charts.filter(item => item !== chart); observers = observers.filter(item => item !== observer);
            note.hidden = true; plot.replaceChildren(); plot.append(node('p',tr('chart.empty')));
          });
        };
        try {
          chart = new uPlot({width:Math.max(260,plot.clientWidth),height:250,tzDate:time => dateForPlot(time,utcDay),
            scales,axes,series,legend:{show:false},plugins:[bootMarkers(selectedBoots,fallback)],
            cursor:{points:{show:false},bind:{dblclick:u => () => u.setScale('x',{min:from,max:to})}},hooks:{setCursor:[updateCursor]}},panel.data,plot);
        } catch (error) { fallback(error); }
        if (chart && !failed) {
          charts.push(chart);
          observer = new ResizeObserver(() => chart.setSize({width:Math.max(260,plot.clientWidth),height:250}));
          observer.observe(plot); observers.push(observer);
          if (selectedBoots.length) {
            const swatch = node('span',undefined,'boot-swatch'); swatch.setAttribute('aria-hidden','true');
            note.append(swatch,node('span',tr('note.bootMarkers'))); section.append(note);
          }
        }
      }
      for (const note of panel.notes) section.append(node('p',tr(note),'chart-note'));
      if (panel.title === 'panel.oom') section.append(node('p',tr(resolution === '1m' ? 'note.oom' : 'note.oomCoarse'),'chart-note'));
    }
    if (resolution !== '1m') {
      section.append(node('p',tr('note.statsCoarse'),'chart-note'));
      section.append(node('p',coarseNotice,'chart-note'));
    }
    if (group.kind === 'disk') {
      const latest = column => {
        const i = prepared.plotted.columns.indexOf(column);
        const unit = column.startsWith('inodes_') ? 'count' : 'bytes';
        return prepared.plotted.units[i] === unit ? prepared.data[i]?.at(-1) : null;
      };
      const usage = diskUsage(latest('used'),latest('avail'),latest('inodes_used'),latest('inodes_total'));
      section.append(node('p',tr('disk.usedRatio') + ': ' + formatValue(usage.used,'%') + ' · ' + tr('disk.inodeRatio') + ': ' + formatValue(usage.inodes,'%'),'traffic'));
    }
    for (const derived of prepared.derived) if (derived.column !== 'swap_free') section.append(node('p',tr('description.' + derived.column),'chart-note'));
    if (group.kind === 'net') for (const text of trafficLines(group.traffic)) section.append(node('p',text,'traffic'));
    const details = node('details'); details.append(node('summary',tr('chart.columns')));
    const table = node('table');
    const lastTime = history.times.at(-1);
    table.append(node('caption',lastTime == null ? tr('chart.empty') : tr('chart.lastPoint',{time:formatTime(lastTime,utcDay)})));
    const head = node('tr');
    for (const label of [tr('chart.column'),tr(resolution === '1m' ? 'stats.last' : prepared.derived.length ? 'stats.meanDerived' : 'stats.mean'),tr('chart.description')]) head.append(node('th',label));
    const thead = node('thead'); thead.append(head); table.append(thead);
    const body = node('tbody');
    for (const column of [...group.columns.slice(1), ...prepared.derived.map(d => d.column)]) {
      const i = prepared.plotted.columns.indexOf(column), row = node('tr'), name = node('th',columnLabel(group.kind,column)); name.scope = 'row'; row.append(name);
      const unit = displayUnit(group.kind,prepared.plotted.units[i]);
      row.append(node('td',formatValue(prepared.data[i]?.at(-1),unit),'latest'));
      const key = 'description.' + column;
      const description = Object.hasOwn(messages[lang],key) ? tr(key) : group.descriptions?.[group.columns.indexOf(column)] || column;
      row.append(node('td',description + (group.kind === 'net' && unit === 'bit/s' ? '; ' + tr('note.net') : ''))); body.append(row);
    }
    table.append(body); details.append(table); section.append(details);
  }
  async function render() {
    if (typeof window.uPlot !== 'function') throw new Error(loadError('uplot'));
    const token = ++generation;
    const to = index.generated_at;
    const seconds = Number(document.querySelector('input[name="period"]:checked').value);
    const resolution = periodPlan(seconds).resolution, from = to - seconds;
    const dataTo = resolution === '1m' ? to : Math.floor(to / 86400) * 86400 - 1;
    el('status').textContent = tr('status.loading');
    el('load-errors').replaceChildren(); el('load-errors').hidden = true;
    const groups = index.groups.filter(group => /^[a-z0-9_-]+$/.test(group.id) && Array.isArray(group.columns) && group.columns[0] === 'time' && Array.isArray(group.units) && group.columns.length === group.units.length);
    const prepared = await Promise.all(groups.map(async group => {
      const keys = historyKeys(group, from, dataTo, resolution);
      const results = await Promise.allSettled(keys.map(loadDay));
      const errors = results.filter(result => result.status === 'rejected').map(result => result.reason.message);
      const files = keys.filter((_, i) => results[i].status === 'fulfilled').map(key => records.get(key));
      return {group, history:buildSeries(files, chartGroup(group, resolution), from, dataTo, to), errors};
    }));
    if (token !== generation) return;
    observers.forEach(observer => observer.disconnect()); observers = [];
    charts.forEach(chart => chart.destroy()); charts = [];
    el('groups').replaceChildren();
    showContents(groups);
    for (const item of prepared) {
      item.errors.forEach(reportError);
      draw(item.group, item.history, from, to, resolution, seconds);
    }
    await showDirectories(token);
    if (token !== generation) return;
    el('status').textContent = groups.length ? tr('status.groups',{count:groups.length}) : tr('status.empty');
  }
  async function start() {
    try {
      document.documentElement.lang = lang;
      for (const element of document.querySelectorAll('[data-i18n]')) element.textContent = tr(element.dataset.i18n);
      await loadScript('index.js').catch(() => { throw new Error(loadError('index')); });
      if (!index || index.schema !== 1 || !Number.isFinite(index.generated_at) || !Array.isArray(index.groups)) throw new Error(tr('error.schema'));
      el('host').textContent = index.host || tr('page.title');
      document.title = (index.host || 'hostdiary') + ' · ' + tr('page.title');
      el('collected').textContent = localTime(index.generated_at);
      el('collected').dateTime = new Date(index.generated_at * 1000).toISOString();
      const boot = latestBoot(index.boots);
      if (boot !== null) {
        el('last-boot').textContent = localTime(boot);
        el('last-boot').dateTime = new Date(boot * 1000).toISOString();
      } else el('last-boot').textContent = tr('page.unavailable');
      showUnavailable();
      el('period').addEventListener('change', () => render().catch(error => reportError(error.message)));
      await render();
    } catch (error) {
      el('status').textContent = tr('error.display');
      reportError(error.message);
    }
  }
  start();
}());
