const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const {bootMarkers, buildSeries: buildHistory, neededDays, displayUnit, displayValue, formatValue, loadError, seriesLabel, axisIncrements, periodPlan, historyKeys, formatTime, trafficLines, coarseNotice} = require('./assets/app.js');
const buildSeries=(...args)=>buildHistory(...args).data;
const {detectLanguage, messages, t} = require('./assets/i18n.js');
const {selectBoots, latestBoot, chartGroup, ratioValue, prepareCharts, panelsFor, stackSeries, mirrorSeries, seriesStats, plotTime, cursorIndex, diskUsage, swapFree, mirrorRange, zeroRange} = require('./assets/views.js');
const visit=node=>[node,...node.children.flatMap(visit)];
const settle=()=>new Promise(resolve=>setImmediate(resolve));

test('boot selection includes window boundaries without mutating input',()=>{
 const boots=Object.freeze([201,200,100,99,150,100,null,'120',NaN,Infinity,120.5,9e12,-9e12]);
 assert.deepEqual(selectBoots(boots,100,200),[100,150,200]);
 assert.deepEqual(selectBoots(boots,100.1,199.9),[150]);
 assert.deepEqual(selectBoots([0,-1,-2],-1,0),[-1,0]);
 assert.deepEqual(selectBoots([100],100,100),[100]);
 assert.deepEqual(selectBoots([100],101,200),[]);
 for(const value of [undefined,null,{},'100',[]]) assert.deepEqual(selectBoots(value,0,200),[]);
 for(const bounds of [[NaN,1],[0,Infinity],[2,1],['0',1]]) assert.throws(()=>selectBoots([], ...bounds),RangeError);
});
test('latest boot uses the last entry, never the maximum',()=>{
 assert.equal(latestBoot(Object.freeze([200,100])),100);
 for(const value of [0,-1,100]) assert.equal(latestBoot([value]),value);
 for(const value of [undefined,null,{},[],[100,null],[100,'200'],[100,NaN],[100,Infinity],[100,1.5],[100,9e12]]) assert.equal(latestBoot(value),null);
});

test('language uses the first valid primary language, never a later preference or prefix',()=>{
 for(const list of [undefined,[],[null,4],['en-US','ja'],['jv'],['jav'],['']]) assert.equal(detectLanguage(list),'en');
 for(const list of [['JA'],['ja-JP'],[null,4,'ja-JP']]) assert.equal(detectLanguage(list),'ja');
});
test('translation falls back and interpolates only supplied parameters as strings',()=>{
 assert.deepEqual(Object.keys(messages.en).sort(),Object.keys(messages.ja).sort());
 assert.equal(t('unknown','stats.min'),messages.en['stats.min']);
 assert.equal(t('ja','unknown.key'),'unknown.key');
 assert.equal(t('en','error.data'),messages.en['error.data']);
 assert.equal(t('en','error.data',{path:42}),messages.en['error.data'].replace('{path}','42'));
 assert.equal(t('ja','test {name}'),'test {name}');
 assert.equal(t('en','{name}',{name:0}),'0');
 assert.equal(t('en','toString'),'toString');
});
test('stack preserves shape and caller data, propagates any missing component, and supports negatives',()=>{
 assert.deepEqual(stackSeries([]),[]);assert.deepEqual(stackSeries([[],[]]),[[],[]]);
 assert.deepEqual(stackSeries([[null],[null]]),[[null],[null]]);
 const input=[[1,2,-2],[3,null,1],[2,4,5]],before=structuredClone(input);
 assert.deepEqual(stackSeries(input),[[1,null,-2],[4,null,-1],[6,null,4]]);
 assert.deepEqual(input,before);
 assert.deepEqual(stackSeries([[1],[2],[3]]),[[1],[3],[6]]);
});
test('mirror negates values including signed zero and preserves null and its input',()=>{
 const input=[null,0,-0,2,-3],result=mirrorSeries(input);
 assert.deepEqual(result,[null,-0,0,-2,3]);assert.deepEqual(input,[null,0,-0,2,-3]);
});
test('statistics ignore missing values and take the last non-null observation',()=>{
 const empty={min:null,avg:null,max:null,last:null,total:null};
 assert.deepEqual(seriesStats([]),empty);assert.deepEqual(seriesStats([null,null]),empty);
 assert.deepEqual(seriesStats([null,4,2,null]),{min:2,avg:3,max:4,last:2,total:null});
 assert.deepEqual(seriesStats([-2,0,4]),{min:-2,avg:2/3,max:4,last:4,total:null});
});
test('coarse statistics weight only finite positive samples; maximum comes from maxima',()=>{
 assert.deepEqual(seriesStats([2,8,null,10,20,30,40],{samples:[1,3,100,0,-1,NaN,null],max:[5,99,null,12,22,32,42],seconds:60}),{min:2,avg:6.5,max:99,last:8,total:1560});
 assert.deepEqual(seriesStats([2,8],{samples:[0,null],max:[5,9],seconds:60}),{min:null,avg:null,max:null,last:null,total:null});
 assert.equal(seriesStats([2,8],{samples:[1,3]}).max,8); // derived: no max array
 assert.equal(seriesStats([2,null],{max:[3,999]}).max,3);
 assert.equal(seriesStats([null],{samples:[4],max:[999],seconds:60}).max,null);
 assert.equal(seriesStats([8,null,16],{seconds:60}).total,1440); // bit/s -> bit, no second conversion
 assert.equal(seriesStats([0],{samples:[1],seconds:60}).total,0);
});
test('plot interval ends and cursor containment use open left, closed right bounds',()=>{
 assert.equal(plotTime(100,300,true),400);assert.equal(plotTime(100,60,false),100);
 for(const [value,want] of [[99,null],[100,null],[101,1],[200,1],[201,2],[300,2],[301,null],[null,null],[NaN,null]]) assert.equal(cursorIndex([100,200,300],value),want);
 assert.equal(cursorIndex([],10),null);assert.equal(cursorIndex([100],100),null);
});
test('disk ratios and swap free require same-row materials and never turn null into zero',()=>{
 assert.deepEqual(diskUsage(20,80,3,4),{used:20,inodes:75});
 assert.deepEqual(diskUsage(0,0,0,0),{used:null,inodes:null});
 assert.deepEqual(diskUsage(null,80,null,4),{used:null,inodes:null});
 assert.equal(diskUsage(2,null,1,null).used,null);
 assert.equal(swapFree(100,30),70);assert.equal(swapFree(0,0),0);
 for(const args of [[null,0],[1,null],[2,3]]) assert.equal(swapFree(...args),null);
});

// Synthetic headers follow the public data-format table, never operational data.
const viewHeaders={
 cpu:[['user','nice','system','iowait','irq','softirq','steal','idle'],Array(8).fill('%')],
 load:[['load1','load5','load15','cpus'],['(none)','(none)','(none)','count']],
 mem:[['total','available','used','buffers','cached','oom_kills'],['bytes','bytes','bytes','bytes','bytes','count']],
 swap:[['total','used','in','out'],['bytes','bytes','bytes/s','bytes/s']],
 procs:[['total','running','sleeping','disk_sleep','stopped','zombie','other'],Array(7).fill('count')],
 tcp:[['established','syn_sent','syn_recv','fin_wait1','fin_wait2','time_wait','close','close_wait','last_ack','listen','closing','other','out_segs','retrans_segs'],[...Array(12).fill('count'),'count/s','count/s']],
 net:[['rx','tx','rx_errors','rx_drops','tx_errors','tx_drops'],['bytes/s','bytes/s',...Array(4).fill('count/s')]],
 disk:[['used','avail','inodes_used','inodes_total'],['bytes','bytes','count','count']],
 diskio:[['read','write','read_ops','write_ops','busy','read_time','write_time'],['bytes/s','bytes/s','count/s','count/s','%','ms/s','ms/s']],
 psi:[['cpu_some','memory_some','memory_full','io_some','io_full'],Array(5).fill('%')],
 custom:[['one','two','three'],['bytes','count','bytes']]
};
const viewGroup=kind=>({kind,columns:['time',...viewHeaders[kind][0]],units:['s',...viewHeaders[kind][1]]});
const expectedPanels={
 cpu:[['stack','%',['user','nice','system','iowait','irq','softirq','steal'],['idle']]],
 load:[['line','(none)',['load1','load5','load15','cpus'],[]]],
 mem:[['stack','bytes',['used','available'],['buffers','cached','total']],['line','count',['oom_kills'],[]]],
 swap:[['stack','bytes',['used','swap_free'],[]],['mirror','bytes/s',['in','out'],[]]],
 procs:[['stack','count',['running','sleeping','disk_sleep','stopped','zombie','other'],['total']]],
 tcp:[['stack','count',viewHeaders.tcp[0].slice(0,12),[]],['line','count/s',['out_segs','retrans_segs'],[]],['line','%',['retrans_ratio'],[]]],
 net:[['mirror','bit/s',['rx','tx'],[]],['line','count/s',['rx_errors','rx_drops','tx_errors','tx_drops'],[]]],
 disk:[['stack','bytes',['used','avail'],[]],['line','count',['inodes_used','inodes_total'],[]]],
 diskio:[['mirror','bytes/s',['read','write'],[]],['mirror','count/s',['read_ops','write_ops'],[]],['line','%',['busy'],[]],['line','ms',['read_completion','write_completion'],[]]],
 psi:[['line','%',viewHeaders.psi[0],[]]],
 custom:[['line','bytes',['one','three'],[]],['line','count',['two'],[]]]
};
for(const kind of Object.keys(viewHeaders)) test('panels and dictionary contracts: '+kind,()=>{
 const input=viewGroup(kind),before=structuredClone(input),panels=panelsFor(input);
 assert.deepEqual(panels.map(p=>[p.mode,p.unit,p.series.map(s=>s.column),p.tableOnly]),expectedPanels[kind]);
 assert.deepEqual(input,before);
 for(const panel of panels){
   assert.ok(panel.series.length);assert.ok(Object.hasOwn(messages.en,panel.title));
   for(const key of panel.notes) assert.ok(Object.hasOwn(messages.en,key),key);
   for(const series of panel.series){
     if(kind!=='custom') assert.ok(Object.hasOwn(messages.en,series.label),series.label);
     assert.match(series.color,/^#[0-9a-f]{6}$/i);
   }
   for(const column of panel.tableOnly) assert.ok(Object.hasOwn(messages.en,'column.'+column));
 }
});
test('OOM panel preserves counts, zeros and missing columns',async()=>{
 const group={...viewGroup('mem'),id:'mem',name:'mem',days:['1970-01-01'],last_seen:200};
 const oom=panelsFor(group).find(panel=>panel.title==='panel.oom');
 assert.ok(oom); assert.equal(oom.unit,'count'); assert.equal(oom.series[0].color,'#8945aa');
 for(const value of [0,null,3]) {
   const elements=await startViewer('memory',[{group,rows:[[100,100,70,30,5,10,value],[200,100,70,30,5,10,value]]}],{generatedAt:200});
   const nodes=visit(elements.get('groups'));
   const row=nodes.find(node=>node.dataset.column==='oom_kills');
   assert.ok(row); assert.equal(row.children[1].textContent,value===null?'—':String(value));
   assert.ok(nodes.some(node=>node.textContent===messages.en['note.oom']));
   assert.equal(elements.plotted.length,value===null?1:2);
   assert.deepEqual(Array.from(elements.plottedData[0][1]),[100,null,100]); // Memory stack and collection gap unchanged.
 }
 const old={...group,columns:group.columns.slice(0,-1),units:group.units.slice(0,-1)};
 assert.equal(panelsFor(old).length,1);
 assert.equal(panelsFor({...group,units:[...group.units.slice(0,-1),'bytes']}).length,1);
 const reordered={...group,columns:['time','oom_kills','used','available'],units:['s','count','bytes','bytes']};
 const prepared=prepareCharts(reordered,[[100],[2],[30],[70]],'1m');
 assert.deepEqual(prepared.panels.find(p=>p.title==='panel.oom').raw.oom_kills,[2]);
 const oldHeader={schema:1,group:'mem',step:60,columns:old.columns,units:old.units};
 const aligned=buildHistory([[oldHeader,[100,100,70,30,5,10]]],group,0,300);
 assert.deepEqual(aligned.data.at(-1),[null]);
});
test('coarse OOM shows mean and maximum, never an interval total',async()=>{
 for(const [resolution,step,period] of [['5m',300,2678400],['1h',3600,31536000],['1d',86400,63072000]]) {
   const group={id:'mem',kind:'mem',name:'mem',columns:['time','oom_kills'],units:['s','count'],days:[],coarse:{[resolution]:[resolution==='1d'?'2026':'2026-10']},last_seen:1793318400};
   const header={schema:1,group:'mem',step,columns:['time','samples','last_time','oom_kills','oom_kills_max'],units:['s','count','s','count','count']};
   const rows=[[1793232000,1,1793232000,0,1],[1793232000+step,3,1793232000+step,0.2,4]];
   for(const language of ['en','ja']) {
     const elements=await startViewer('memory',[{group,header,rows}],{generatedAt:1793448000,period:String(period),languages:[language]});
     const nodes=visit(elements.get('groups')),row=nodes.find(node=>node.dataset.column==='oom_kills');
     assert.deepEqual(row.children.slice(1,5).map(node=>node.textContent),['0','0.15','4','0.2']);
     assert.equal(row.children.length,6); // No total column.
     assert.ok(nodes.some(node=>node.textContent===messages[language]['note.oomCoarse']));
     assert.match(messages.en['note.oomCoarse'],/positive maximum/);
     assert.match(messages.ja['note.oomCoarse'],/平均が0でも最大が正/);
     assert.deepEqual(Array.from(elements.plottedData[0][1]),[0,0.2]);
     assert.deepEqual(Array.from(elements.plottedData[0][2]),[1,4]);
     const chart=elements.instances[0];chart.cursor.left=1793232000+2*step;
     elements.plotted[0].hooks.setCursor[0](chart);
     assert.equal(row.children[5].textContent,'0.2');
   }
 }
});
test('panels omit absent columns and empty panels, preserve one unit, fixed colors and style exceptions',()=>{
 for(const kind of Object.keys(viewHeaders)) assert.deepEqual(panelsFor({kind,columns:['time'],units:['s']}),[]);
 const mem=viewGroup('mem');mem.columns.splice(2,1);mem.units.splice(2,1);
 assert.deepEqual(panelsFor(mem)[0].series.map(s=>s.column),['used']);
 const swap={kind:'swap',columns:['time','used'],units:['s','bytes']};assert.deepEqual(panelsFor(swap)[0].series.map(s=>s.column),['used']);
 assert.deepEqual(panelsFor({kind:'diskio',columns:['time','read_time'],units:['s','ms/s']}),[]);
 assert.deepEqual(panelsFor({kind:'tcp',columns:['time','out_segs'],units:['s','count/s']}).flatMap(p=>p.series.map(s=>s.column)),['out_segs']);
 const net=panelsFor(viewGroup('net'));assert.equal(net[0].totalUnit,'bit');
 assert.equal(net[0].series[0].color,'#008800');assert.equal(net[0].series[1].color,'#cc2222');
 assert.ok(net[1].series.every(s=>s.fill===false));
 assert.ok(panelsFor(viewGroup('load'))[0].series.at(-1).dash.length);
 assert.equal(panelsFor(viewGroup('disk'))[1].series.at(-1).fill,false);
 assert.equal(panelsFor(viewGroup('cpu'))[0].series[0].color,'#245ac6');
 assert.deepEqual(panelsFor(viewGroup('cpu'))[0].range,[0,100]);
 const input=panelsFor(viewGroup('mem'));assert.equal(stackSeries([[30],[70]]).at(-1)[0],100);
 assert.equal(input[0].series.length,2);
});
test('PSI fixed colors survive missing or reordered columns; TCP other remains English',()=>{
 const all=panelsFor(viewGroup('psi'))[0].series;
 const partial=panelsFor({kind:'psi',columns:['time','io_full','memory_some'],units:['s','%','%']})[0].series;
 for(const series of partial) assert.equal(series.color,all.find(s=>s.column===series.column).color);
 const other=panelsFor(viewGroup('tcp'))[0].series.find(s=>s.column==='other');
 assert.equal(t('ja',other.label),'other');
 assert.equal(t('ja','column.other'),'その他');
});
test('pure transformations reject incompatible shapes at their entry points',()=>{
 assert.throws(()=>stackSeries([[1],[2,3]]),TypeError);
 assert.throws(()=>stackSeries([1]),TypeError);
 assert.throws(()=>seriesStats([1],{samples:[]}),TypeError);
 assert.throws(()=>seriesStats([1],{max:[]}),TypeError);
 assert.throws(()=>panelsFor({kind:'cpu',columns:['time','user'],units:['s']}),TypeError);
 assert.throws(()=>panelsFor({kind:'cpu',columns:['user'],units:['%']}),TypeError);
});

test('B boundaries: language, invalid values, statistics, duplicate cursor boundaries and English derived labels',()=>{
 assert.equal(detectLanguage(['',null,'ja-JP']),'ja');
 assert.deepEqual(stackSeries([[1,NaN,undefined,Infinity],[2,3,4,5]]),[[1,null,null,null],[3,null,null,null]]);
 assert.deepEqual(mirrorSeries([NaN,undefined,Infinity,null]),[null,null,null,null]);
 assert.deepEqual(seriesStats([NaN,3,4],{samples:[1,1,0],max:[99,null,999]}),{min:3,avg:3,max:null,last:3,total:null});
 assert.equal(cursorIndex([100,200,200,300],200),1);assert.equal(cursorIndex([100,200,200,300],201),3);
 for(const [column,label] of Object.entries({swap_free:'Free',read_completion:'Read completion',write_completion:'Write completion',retrans_ratio:'Retransmission ratio'})) assert.equal(t('en','column.'+column),label);
});
test('B known panels omit wrong units, including table-only columns and derived materials',()=>{
 for(const kind of Object.keys(viewHeaders).filter(k=>k!=='custom')) {
   const g=viewGroup(kind);g.units=g.units.map((unit,i)=>i?'wrong':unit);
   assert.deepEqual(panelsFor(g),[]);
 }
 const g=viewGroup('mem');g.units[1]='count';assert.ok(!panelsFor(g)[0].tableOnly.includes('total'));
 const tcp=viewGroup('tcp');tcp.units[tcp.units.length-1]='count';
 assert.ok(!panelsFor(tcp).some(p=>p.title==='panel.retrans'));
 const future={kind:'toString',columns:['time','swap_free'],units:['s','count']};
 assert.equal(panelsFor(future)[0].unit,'count');assert.match(panelsFor(future)[0].series[0].color,/^#[0-9a-f]{6}$/);
});
test('B coarse builder returns separate samples, shifts interval ends before gaps and rejects invalid steps',()=>{
 const g={id:'fixture',kind:'load',columns:['time','load1','load1_max'],units:['s','(none)','(none)']};
 const h={schema:1,group:'fixture',step:300,columns:['time','samples','last_time','load1','load1_max'],units:['s','count','s','(none)','(none)']};
 assert.deepEqual(buildHistory([[h,[0,5,240,1,3],[600,2,660,2,4]]],g,0,600),{data:[[300,600,900],[1,null,2],[3,null,4]],samples:[5,null,2],times:[0,null,600]});
 for(const step of [undefined,0,-1,NaN,Infinity]) assert.deepEqual(buildHistory([[{...h,step},[0,5,0,1,3]]],g,0,600),{data:[[],[],[]],samples:[],times:[]});
});
test('B network totals integrate observed rates only, including a week at 100 Mbit/s',()=>{
 assert.equal(formatValue(seriesStats([1e8,null,1e8],{seconds:60}).total,'bit'),'12 Gbit');
 assert.equal(formatValue(seriesStats([1e8,null,1e8],{samples:[5,999,2],seconds:60}).total,'bit'),'42 Gbit');
 assert.equal(formatValue(seriesStats(Array(10080).fill(1e8),{seconds:60}).total,'bit'),'60.48 Tbit');
 assert.equal(formatValue(1e15,'bit'),'1 Pbit');
 assert.equal(formatValue(3,'count'),'3');assert.equal(formatValue(3,'count/s'),'3');
});

const group = {id:'net-eth0-test', kind:'net', columns:['time','rx','tx'], units:['s','bytes/s','bytes/s']};
const header = {schema:1, group:group.id, columns:group.columns, units:group.units, step:60};

test('directory days use exact seven UTC days or oldest, independent of ordering',()=>{
 const {directoryDays}=require('./assets/app.js');
 assert.deepEqual(directoryDays(['2026-11-01','2026-10-20','2026-10-25','invalid','2026-02-30']),['2026-11-01','2026-10-25']);
 assert.deepEqual(directoryDays(['2026-10-20','2026-11-01','2026-10-26']),['2026-11-01','2026-10-20']);
 assert.deepEqual(directoryDays([]),[]);
 assert.deepEqual(directoryDays(['2026-11-01']),['2026-11-01']);
 assert.deepEqual(directoryDays(['2026-11-01','2026-11-01']),['2026-11-01']);
});
test('directory comparison matches mount and path; absence remains missing; signed binary changes',()=>{
 const {directoryTables,formatDifference}=require('./assets/app.js');
 const latest={filesystems:[{mount:'/',total:4096,complete:false,errors:3,entries:[{path:'/a',depth:1,bytes:1024},{path:'/new',depth:1,bytes:0}]}]};
 const old={filesystems:[{mount:'/',entries:[{path:'/a',bytes:2048}]}]};
 const result=directoryTables(latest,old);
 assert.equal(result[0].status,'partial (3 entries skipped)');
 assert.deepEqual(result[0].rows.map(r=>[r.previous,r.difference]),[[2048,-1024],[null,null]]);
 assert.equal(directoryTables(latest,{filesystems:[{mount:'/other',entries:[{path:'/a',bytes:99}]}]})[0].rows[0].previous,null);
 assert.equal(formatDifference(-1024),'-1 KiB');assert.equal(formatDifference(1024),'+1 KiB');assert.equal(formatDifference(0),'0 B');assert.equal(formatDifference(null),'—');
});

test('partial directory status omits nonnumeric skipped counts',()=>{
 const {directoryTables}=require('./assets/app.js');
 for(const errors of [undefined,null,'3',NaN,Infinity])assert.equal(directoryTables({filesystems:[{complete:false,errors,entries:[]}]})[0].status,'partial');
 assert.equal(directoryTables({filesystems:[{complete:false,errors:0,entries:[]}]})[0].status,'partial (0 entries skipped)');
});

// Ratios use matched materials from one row. Missing data never coerces to zero.
test('derived ratios require finite operands and a positive denominator',()=>{
 assert.equal(ratioValue(6,2,1),3);
 assert.equal(ratioValue(2,10,100),20);
 assert.equal(ratioValue(0,10,100),0);
 for(const [n,d] of [[null,1],[undefined,1],[NaN,1],[Infinity,1],[1,null],[1,0],[1,-1],[1,NaN],[1,Infinity],['2',1],[1,'2']]) assert.equal(ratioValue(n,d,1),null);
 assert.equal(ratioValue(Number.MAX_VALUE,Number.MIN_VALUE,100),null);
});
test('coarse ratios hide rounded-away means and never divide maxima',()=>{
 assert.equal(ratioValue(0,2,1,1,9,true),null);
 assert.equal(ratioValue(2,0,1,9,1,true),null);
 assert.equal(ratioValue(0,2,1,0,9,true),0);
 assert.equal(ratioValue(0,2,1,null,null,true),0);
 assert.equal(ratioValue(0,2,1,9,9,false),0);
 assert.equal(ratioValue(6,2,1,900,1,true),3);
});
const diskGroup={id:'diskio-fixture',kind:'diskio',name:'fixture',columns:['time','read','write','read_ops','write_ops','busy','read_time','write_time'],units:['s','bytes/s','bytes/s','count/s','count/s','%','ms/s','ms/s'],days:['1970-01-01'],last_seen:100};
test('disk completion lines use the same row with missing old columns and gaps',()=>{
 const h={schema:1,group:diskGroup.id,step:60,columns:diskGroup.columns,units:diskGroup.units};
 const old={...h,columns:h.columns.slice(0,6),units:h.units.slice(0,6)};
 const raw=buildSeries([[old,[0,1,2,3,4,5],h,[60,10,20,2,4,30,6,20],[240,10,20,0,1,30,null,0]]],diskGroup,0,240);
 const charts=prepareCharts(diskGroup,raw,'1m');
 const ratio=charts.panels.at(-1);
 assert.deepEqual(ratio.group.columns,['time','read_completion','write_completion']);
 assert.deepEqual(ratio.group.units,['s','ms','ms']);
 assert.deepEqual(ratio.data,[[0,60,150,240],[null,3,null,null],[null,5,null,0]]);
 assert.deepEqual(raw[0],[0,60,150,240]); // caller-owned materials are unchanged
 assert.equal(raw.length,8);
 for(const panel of charts.panels) assert.ok(new Set(panel.group.units.slice(1)).size<=2);
});
test('coarse TCP ratio uses means, preserves gaps and has no ratio maximum',()=>{
 const g={id:'tcp',kind:'tcp',columns:['time','established','out_segs','retrans_segs'],units:['s','count','count/s','count/s']};
 const plotted=chartGroup(g,'5m');
 const data=[[0,300,600],[1,1,1],[9,9,9],[2,2,2],[20,20,20],[0,0,1],[1,0,99]];
 const charts=prepareCharts(g,data,'5m'),ratio=charts.panels.at(-1);
 assert.deepEqual(ratio.group.columns,['time','retrans_ratio']);
 assert.deepEqual(ratio.data,[[0,300,600],[null,0,50]]);
 assert.equal(charts.plotted.columns.filter(c=>c==='retrans_ratio').length,1);
 assert.ok(!charts.plotted.columns.includes('retrans_ratio_max'));
 assert.deepEqual(charts.panels[0].group.columns,['time','established']);
 assert.deepEqual(charts.panels[1].group.columns,['time','out_segs','retrans_segs','out_segs_max','retrans_segs_max']);
 const missing=prepareCharts(g,[[0],[1],[null],[2]],'1m');
 assert.equal(missing.panels.length,2);
 assert.deepEqual(missing.data.at(-1),[null]);
});
test('derived panels require a non-null value; measured zero still creates a panel',()=>{
 const tcp={id:'tcp',kind:'tcp',columns:['time','out_segs','retrans_segs'],units:['s','count/s','count/s']};
 for(const data of [[[],[],[]],[[0,60],[0,0],[0,0]],[[0],[null],[1]]]) assert.equal(prepareCharts(tcp,data,'1m').panels.length,1);
 assert.equal(prepareCharts(tcp,[[0],[2],[0]],'1m').panels.length,2);
 const diskData=[[0],[1],[2],[0],[0],[3],[0],[0]];
 assert.equal(prepareCharts(diskGroup,diskData,'1m').panels.length,3);
 const mixed=diskData.map(a=>[...a]);mixed[3]=[1];
 assert.equal(prepareCharts(diskGroup,mixed,'1m').panels.length,4);
 const coarse=chartGroup(tcp,'5m');
 assert.deepEqual(coarse.columns,['time','out_segs','out_segs_max','retrans_segs','retrans_segs_max']);
 assert.equal(prepareCharts(tcp,[[0],[1],[10],[0],[1]],'5m').panels.length,1);
});

test('long periods choose coarse files by UTC month/year overlap',()=>{
 const to=Date.parse('2026-11-01T12:00:00Z')/1000;
 const g={...group,days:['2026-11-01'],coarse:{'5m':['2026-09','2026-10','2026-11','2026-13','../bad'],'1h':['2025-11','2026-10'],'1d':['2024','2025','2026','notes']}};
 assert.equal(periodPlan(3600).resolution,'1m');
 assert.equal(periodPlan(2678400).resolution,'5m');
 assert.equal(periodPlan(31536000).resolution,'1h');
 assert.equal(periodPlan(63072000).resolution,'1d');
 assert.deepEqual(historyKeys(g,to-2678400,to,'5m'),['5m/2026-10/'+g.id,'5m/2026-11/'+g.id]);
 assert.deepEqual(historyKeys(g,to-63072000,to,'1d'),['1d/2024/'+g.id,'1d/2025/'+g.id,'1d/2026/'+g.id]);
 assert.deepEqual(historyKeys(g,to-86400,to,'1m'),['1m/2026-11-01/'+g.id]);
 assert.deepEqual(historyKeys({...g,coarse:{}},0,to,'5m'),[]);
});

test('coarse plots include mean/max pairs, never samples or last_time',()=>{
 const g=chartGroup(group,'5m');
 assert.deepEqual(g.columns,['time','rx','rx_max','tx','tx_max']);
 const h={...header,step:300,columns:['time','samples','last_time','rx','rx_max','tx','tx_max'],units:['s','count','s','bytes/s','bytes/s','bytes/s','bytes/s']};
 assert.deepEqual(buildSeries([[h,[0,5,240,2,4,null,null],[600,1,600,3,9,4,5]]],g,0,600),[[300,600,900],[16,null,24],[32,null,72],[null,null,32],[null,null,40]]);
 assert.equal(formatTime(0,true),'1970-01-01 (UTC day)');
 assert.equal(coarseNotice,'Coarse values cover completed UTC days. See 1 day for today. If recent days are missing, check 1 day / 1 week.');
});

test('traffic text names binary units, UTC estimates and missing days',()=>{
 assert.deepEqual(trafficLines({month:{rx:2**30,tx:1024,days_missing:2},last30d:{rx:0,tx:2048,days_missing:0}}),[
 'This month (UTC, estimate): rx 1 GiB / tx 1 KiB · 2 days missing',
 'Last 30 days (estimate): rx 0 B / tx 2 KiB']);
 assert.deepEqual(trafficLines(null),[]);
 const html=fs.readFileSync(require.resolve('./index.html'),'utf8');
 for(const text of ['1 month','1 year','2 years']) assert.ok(html.includes(text));
});

test('all-missing traffic is no data; one missing day is singular',()=>{
 assert.deepEqual(trafficLines({month:{rx:99,tx:0,days:1,days_missing:1},last30d:{rx:1024,tx:0,days:30,days_missing:1}}),[
 'This month (UTC, estimate): no data · 1 day missing',
 'Last 30 days (estimate): rx 1 KiB / tx 0 B · 1 day missing']);
 assert.ok(trafficLines({month:{rx:0,tx:0,days:1,days_missing:2},last30d:{rx:0,tx:0,days:30,days_missing:30}}).every(text=>text.includes('no data') && !text.includes(' B')));
});

test('rounding promotes byte units rather than rendering 1,024 of the smaller unit',()=>{
 assert.equal(formatValue(1023.9996*2**30,'bytes'),'1 TiB');
 assert.equal(formatValue(1023.9996,'bytes/s'),'1 KiB/s');
 assert.equal(formatValue(1023.9994,'bytes'),'1,023.999 B');
});

// Valid observations stay numeric, null stays a gap, and only network bytes/s
// becomes bit/s. File headers determine the meaning of every following row.
test('network conversion and null preservation', () => {
 assert.deepEqual(buildSeries([[header,[100,2,null],[160,4,8]]],group,100,160),[[100,160],[16,32],[null,64]]);
 assert.equal(displayValue('diskio','bytes/s',2),2);
 assert.equal(displayUnit('net','bytes/s'),'bit/s');
 assert.equal(displayValue('net','bytes/s',null),null);
});
test('a 30 minute gap inserts a null across all columns', () => {
 assert.deepEqual(buildSeries([[header,[100,1,2],[1900,3,4]]],group,100,1900),[[100,1000,1900],[8,null,24],[16,null,32]]);
});
test('step times 1.5 is a strict boundary', () => {
 assert.equal(buildSeries([[header,[100,1,2],[190,3,4]]],group,100,190)[0].length,2);
 assert.equal(buildSeries([[header,[100,1,2],[191,3,4]]],group,100,191)[0].length,3);
});
test('changed column order, invalid rows, and a repeated timestamp', () => {
 const changed={...header,columns:['time','tx','rx']};
 assert.deepEqual(buildSeries([[header,[100,1,2],{...header,schema:2},[120,9,9],changed,[160,3,4],[160,5,6],[170,'bad',7]]],group,100,160),[[100,160],[8,48],[16,40]]);
});
test('headers do not leak across files; old columns align to current metadata', () => {
 const old={...header,columns:['time','rx'],units:['s','bytes/s']};
 assert.deepEqual(buildSeries([[old,[100,2]],[[160,99,99],header,[220,3,4]]],group,100,220),[[100,160,220],[16,null,24],[null,null,32]]);
});
test('window filters observations and sorts timestamps', () => {
 assert.deepEqual(buildSeries([[header,[160,2,3],[100,1,2],[220,4,5]]],group,100,160),[[100,160],[8,16],[16,24]]);
});
test('UTC day boundary and malformed path days', () => {
 const end=Date.parse('2026-11-01T00:00:00Z')/1000;
 assert.deepEqual(neededDays(['2026-10-30','2026-10-31','2026-11-01','../bad','2026-02-30'],end-1,end),['2026-10-31','2026-11-01']);
});

test('byte formatting uses binary units at exact boundaries', () => {
 for (const [value,expected] of [[0,'0 B'],[1023,'1,023 B'],[1024,'1 KiB'],[2**30,'1 GiB'],[2**40,'1 TiB']]) assert.equal(formatValue(value,'bytes'),expected);
 assert.equal(formatValue(1024,'bytes/s'),'1 KiB/s');
});
test('network formatting uses decimal bit rates after conversion', () => {
 for (const [value,expected] of [[0,'0 bit/s'],[999,'999 bit/s'],[1000,'1 kbit/s'],[1e6,'1 Mbit/s'],[1e9,'1 Gbit/s']]) assert.equal(formatValue(value,'bit/s'),expected);
 assert.equal(formatValue(displayValue('net','bytes/s',125),'bit/s'),'1 kbit/s');
});
test('counts and percentages use ordinary numbers with at most three decimals', () => {
 assert.equal(formatValue(1234567890,'count'),'1,234,567,890');
 assert.equal(formatValue(12.34567,'%'),'12.346 %');
 assert.equal(formatValue(1.23456,'(none)'),'1.235');
 assert.equal(formatValue(-0,'bytes'),'0 B');
});
test('missing or invalid nonnegative observations have no unit', () => {
 for (const value of [null,undefined,NaN,Infinity,-1]) for (const unit of ['bytes','bytes/s','bit/s','count','%']) assert.equal(formatValue(value,unit),'—');
});
test('load failures have source-specific English messages', () => {
 assert.equal(loadError('index'),'Collection index unavailable. Restore index.js beside index.html.');
 assert.equal(loadError('uplot'),'Chart library unavailable. Restore assets/vendor/uplot-1.6.32/.');
 assert.equal(loadError('data','data/1m/2026-10-31/cpu.js'),'History file unavailable: data/1m/2026-10-31/cpu.js. Restore this data file.');
});

test('unitless series have no unit in their legend or values',()=>{
 assert.equal(seriesLabel('load1','(none)'),'load1');
 assert.equal(formatValue(1.25,'(none)'),'1.25');
 assert.equal(seriesLabel('rx','bit/s'),'rx (bit/s)');
});
test('byte axis increments are powers of two across byte units',()=>{
 for(const unit of ['bytes','bytes/s']) {
   const increments=axisIncrements(unit);
   assert.ok(increments.length>0);
   assert.ok(increments.every(value=>value>0 && Number.isInteger(Math.log2(value))));
   assert.ok(increments.slice(1).every((value,i)=>value===increments[i]*2));
   for(const value of [1,2,4,8,1024,2048,4096,8192,2**20,2**30,2**40]) assert.ok(increments.includes(value));
 }
 assert.equal(axisIncrements('bit/s'),undefined);
 assert.equal(axisIncrements('count'),undefined);
 assert.equal(axisIncrements('(none)'),undefined);
});

// Exercise the loader's error routing with a minimal DOM, without a browser or
// external modules. This verifies messages, not actual layout/canvas rendering.
async function startViewer(failure, supplied, config={}) {
 const elements = new Map();
 const element = tag => ({tagName:String(tag).toUpperCase(),children:[],textContent:'',dataset:{},style:{},clientWidth:400,listeners:{},append(...nodes){this.children.push(...nodes);},replaceChildren(){this.children=[];},remove(){},setAttribute(key,value){this[key]=value;},addEventListener(type,fn){this.listeners[type]=fn;}});
 const sandbox = {Intl, Date, Map, Set, Promise, Number, Error};
 const plotted = [];
 const plottedData = [], legendRows = [];
 const instances=[],deferred=[];
 const consoleErrors=[],resizeObservers=[];
 sandbox.console={error(...args){consoleErrors.push(args);}};
 let period=config.period || (failure==='coarse'?'63072000':failure==='coarse5m'?'2678400':'86400');
 const staticElements=[...fs.readFileSync(require.resolve('./index.html'),'utf8').matchAll(/data-i18n="([^"]+)"/g)].map(match=>({...element(),dataset:{i18n:match[1]}}));
 sandbox.navigator={languages:config.languages || ['en-US'],language:'en-US'};
 const loadGroup={id:'load',kind:'load',name:'load',columns:['time','load1','load5','load15','cpus'],units:['s','(none)','(none)','(none)','count'],days:['1970-01-01'],last_seen:100};
 sandbox.window = sandbox;
 sandbox.document = {
   documentElement:{lang:'en'},querySelectorAll(){return staticElements;},
   getElementById(id){if(!elements.has(id)) elements.set(id,element());return elements.get(id);},
   createElement:element,
   querySelector(){return {value:period};},
   head:{append(script){const run=() => {
     if(config.fail?.(script.src)) return script.onerror();
     if(script.src==='index.js' && failure!=='index') {
       const isCoarse=failure==='coarse'||failure==='coarse5m';
       const groups=supplied ? supplied.map(item=>item.group) : isCoarse?[{...loadGroup,days:[],coarse:{'1d':['2026'],'5m':['2026-10']}}]:failure==='load'?[loadGroup]:failure==='data'?[{id:'cpu',kind:'cpu',name:'cpu',columns:['time','user'],units:['s','%'],days:['1970-01-01'],last_seen:100}]:[];
       sandbox.HD.index({schema:1,host:'fixture-host',generated_at:config.generatedAt ?? (isCoarse?Date.parse('2026-10-31T12:00:00Z')/1000:100),boots:config.boots,unavailable:config.unavailable || [],groups,...(failure.startsWith('dirs')?{dirs:{days:failure==='dirs-single'?['2026-11-01']:['2026-10-25','2026-11-01']}}:{})});
       script.onload();
     } else if((failure==='dirs'||failure==='dirs-single'||failure==='dirs-missing-errors') && script.src.startsWith('data/dirs/')) {
       const day=script.src.slice(10,-3),latest=day==='2026-11-01';
       sandbox.HD.dirs(day,{schema:1,kind:'dirs',filesystems:[{mount:'/',total:4096,complete:failure==='dirs-missing-errors'?false:!latest,errors:failure==='dirs-missing-errors'?undefined:latest?3:0,entries:latest?[{path:'/<script>',depth:1,bytes:1024},{path:'/new',depth:1,bytes:0}]:[{path:'/<script>',depth:1,bytes:2048}]}]});
       script.onload();
     } else if(supplied && script.src.startsWith('data/')) {
       const item=supplied.find(item=>script.src.endsWith('/'+item.group.id+'.js'));
       const key=script.src.slice(5,-3);
       sandbox.HD.l(key,item.header || {schema:1,group:item.group.id,columns:item.group.columns,units:item.group.units,step:60});
       for(const row of item.rows) sandbox.HD.l(key,row);
       script.onload();
     } else if((failure==='coarse'||failure==='coarse5m') && script.src.startsWith('data/')) {
       const key=failure==='coarse'?'1d/2026/load':'5m/2026-10/load';
       sandbox.HD.l(key,{schema:1,group:'load',step:failure==='coarse'?86400:300,columns:['time','samples','last_time','load1','load1_max','load5','load5_max','load15','load15_max','cpus','cpus_max'],units:['s','count','s','(none)','(none)','(none)','(none)','(none)','(none)','count','count']});
       sandbox.HD.l(key,[1793318400,1440,1793404740,1,9,2,8,3,7,4,6]);
       sandbox.HD.l(key,[1793404800,1,1793404800,99,99,99,99,99,99,99,99]);
       script.onload();
     } else if(failure==='load' && script.src.startsWith('data/')) {
       sandbox.HD.l('1m/1970-01-01/load',{schema:1,group:'load',columns:loadGroup.columns,units:loadGroup.units,step:60});
       sandbox.HD.l('1m/1970-01-01/load',[100,1,2,3,4]);
       script.onload();
     } else script.onerror();
   }; if(config.hold?.(script.src)) deferred.push(run);else queueMicrotask(run);}}
 };
 if(failure!=='uplot') {
   sandbox.uPlot=function(options,data){
     plotted.push(options);plottedData.push(data);
     const empty=data.slice(1).every(column=>column.every(value=>value===null));
     if(empty && config.failEmpty==='constructor') throw new Error('fixture empty construction failure');
     const rows=options.series.map(()=>({style:{}}));legendRows.push(rows);
     Object.assign(this,markerChart(options.scales.x.min,options.scales.x.max,config.ratio || 1));
     this.root={querySelectorAll:()=>rows};this.data=data;this.cursor={left:-10};this.posToVal=value=>value;
     this.draw=()=>{for(const plugin of options.plugins || [])for(const hook of plugin.hooks.draw || [])hook(this);};
     this.setScale=(_key,range)=>{Object.assign(this.scales.x,range);this.draw();};
     this.setSize=({width})=>{this.width=width;this.ctx.canvas.width=width*(config.ratio || 1);this.bbox.width=(width-40)*(config.ratio || 1);this.draw();};
     this.destroy=()=>{this.destroyed=true;};
     if(empty && config.failEmpty==='draw')this.valToPos=()=>{throw new Error('fixture empty drawing failure');};
     instances.push(this);Promise.resolve().then(()=>{if(!this.destroyed)this.draw();});
   };
   sandbox.uPlot.paths={stepped:options=>({stepped:options})};
 }
 sandbox.ResizeObserver=class {constructor(callback){this.callback=callback;resizeObservers.push(this);}observe(){} disconnect(){this.disconnected=true;}};
 for(const asset of ['i18n','views','app']) vm.runInNewContext(fs.readFileSync(require.resolve('./assets/'+asset+'.js'),'utf8'),sandbox);
 await new Promise(resolve=>setImmediate(resolve));
 elements.plotted=plotted;
 elements.plottedData=plottedData;elements.legendRows=legendRows;
 elements.instances=instances;elements.sandbox=sandbox;elements.staticElements=staticElements;elements.deferred=deferred;
 elements.consoleErrors=consoleErrors;elements.resizeObservers=resizeObservers;
 elements.changePeriod=value=>{period=String(value);elements.get('period').listeners.change();};
 return elements;
}

// Canvas recorder checks coordinates and state, not real pixels or uPlot layout.
function markerChart(from=100,to=300,ratio=1) {
 const calls=[],positions=[],saved=[];
 const ctx={canvas:{width:400*ratio},strokeStyle:'original',lineWidth:9,
   save(){saved.push({strokeStyle:this.strokeStyle,lineWidth:this.lineWidth});calls.push(['save']);},
   restore(){Object.assign(this,saved.pop());calls.push(['restore']);},
   beginPath(){calls.push(['begin']);},rect(...args){calls.push(['rect',...args]);},clip(){calls.push(['clip']);},
   setLineDash(dash){calls.push(['dash',...dash]);},moveTo(...args){calls.push(['move',...args]);},lineTo(...args){calls.push(['line',...args]);},
   stroke(){calls.push(['stroke',this.strokeStyle,this.lineWidth]);}};
 const u={width:400,ctx,calls,positions,scales:{x:{min:from,max:to}},bbox:{left:20*ratio,top:10*ratio,width:360*ratio,height:200*ratio}};
 u.valToPos=(value,scale,canvas)=>{positions.push([value,scale,canvas]);return u.bbox.left+(value-u.scales.x.min)/(u.scales.x.max-u.scales.x.min)*u.bbox.width;};
 return u;
}
test('boot markers clip canvas lines and restore drawing state',()=>{
 for(const ratio of [1,2]) {
   const u=markerChart(100,300,ratio);
   bootMarkers([300,200,100,200,50,350]).hooks.draw[0](u);
   assert.deepEqual(u.positions,[[100,'x',true],[200,'x',true],[300,'x',true]]);
   assert.deepEqual(u.calls.filter(c=>c[0]==='move'),[['move',20*ratio,10*ratio],['move',200*ratio,10*ratio],['move',380*ratio,10*ratio]]);
   assert.deepEqual(u.calls.filter(c=>c[0]==='line'),[['line',20*ratio,210*ratio],['line',200*ratio,210*ratio],['line',380*ratio,210*ratio]]);
   assert.deepEqual(u.calls.find(c=>c[0]==='rect'),['rect',20*ratio,10*ratio,360*ratio,200*ratio]);
   assert.deepEqual(u.calls.find(c=>c[0]==='dash'),['dash',3*ratio,3*ratio]);
   assert.deepEqual(u.calls.find(c=>c[0]==='stroke'),['stroke','#53677f',ratio]);
   assert.equal(u.calls[0][0],'save');assert.equal(u.calls.at(-1)[0],'restore');
   assert.ok(u.calls.findIndex(c=>c[0]==='clip')<u.calls.findIndex(c=>c[0]==='move'));
   assert.equal(u.ctx.lineWidth,9);assert.equal(u.ctx.strokeStyle,'original');
 }
});
test('boot markers skip invalid scale ranges without breaking charts',()=>{
 const plugin=bootMarkers([100,200,300]);
 for(const bounds of [[NaN,300],[100,Infinity],[300,100],[undefined,undefined]]) {
   const u=markerChart(...bounds);Object.assign(u.scales.x,{min:bounds[0],max:bounds[1]});
   assert.doesNotThrow(()=>plugin.hooks.draw[0](u));assert.deepEqual(u.calls,[]);
 }
 const u=markerChart(100.5,299.5);plugin.hooks.draw[0](u);
 assert.deepEqual(u.positions,[[200,'x',true]]);
 const empty=markerChart();bootMarkers([]).hooks.draw[0](empty);assert.deepEqual(empty.calls,[]);
});
test('boot markers follow zoom and resize without changing statistics',async()=>{
 const group={id:'load',kind:'load',name:'load',columns:['time','load1'],units:['s','(none)'],days:['1970-01-01'],last_seen:300};
 const e=await startViewer('extra',[{group,rows:[[100,1],[160,2],[300,3]]}],{generatedAt:300,period:'3600',boots:[100,200,300]});
 const chart=e.instances[0],row=visit(e.get('groups')).find(n=>n.dataset.column==='load1');
 const before=row.children.map(n=>n.textContent);
 assert.deepEqual(chart.positions.map(p=>p[0]),[100,200,300]);
 chart.positions.length=0;chart.setScale('x',{min:150,max:250});assert.deepEqual(chart.positions.map(p=>p[0]),[200]);
 chart.positions.length=0;chart.setSize({width:640});assert.deepEqual(chart.positions.map(p=>p[0]),[200]);
 chart.positions.length=0;e.plotted[0].cursor.bind.dblclick(chart)();assert.deepEqual(chart.positions.map(p=>p[0]),[100,200,300]);
 assert.deepEqual(row.children.map(n=>n.textContent),before);
 assert.deepEqual(Array.from(e.plottedData[0][1]),[1,2,null,3]);
});
test('boot markers remain visible across missing and empty history',async()=>{
 const group={id:'load',kind:'load',name:'load',columns:['time','load1'],units:['s','(none)'],days:['1970-01-01'],last_seen:300};
 for(const rows of [[],[[100,null],[300,null]],[[100,1],[300,2]]]) {
   const e=await startViewer('extra',[{group,rows}],{generatedAt:300,period:'3600',boots:[200]});
   assert.equal(e.instances.length,1);assert.deepEqual(e.instances[0].positions.map(p=>p[0]),[200]);
   assert.ok(visit(e.get('groups')).some(n=>n.textContent===messages.en['note.bootMarkers']));
 }
 for(const boots of [undefined,[],[-4000,301]]) {
   const e=await startViewer('extra',[{group,rows:[]}],{generatedAt:300,period:'3600',boots});
   assert.equal(e.instances.length,0);assert.ok(!visit(e.get('groups')).some(n=>n.className==='boot-note'));
 }
 const e=await startViewer('coarse',undefined,{boots:[1793440000]});
 assert.ok(e.instances.length>0);
 for(const chart of e.instances)assert.deepEqual(chart.positions.map(p=>p[0]),[1793440000]); // In today's unobserved coarse region.
 const mem={...viewGroup('mem'),id:'mem',name:'mem',days:['1970-01-01'],last_seen:100};
 const all=await startViewer('extra',[{group:mem,rows:[]}],{boots:[100]});
 assert.equal(all.instances.length,2); // Both memory and OOM receive markers without observations.
 for(const chart of all.instances)assert.deepEqual(chart.positions.map(p=>p[0]),[100]);
});
test('empty chart failure falls back without stopping other panels',async()=>{
 const group={...viewGroup('mem'),id:'mem',name:'mem',days:['1970-01-01'],last_seen:100};
 for(const failure of ['constructor','draw']) {
   const e=await startViewer('extra',[{group,rows:[[100,null,null,null,null,null,1]]}],{boots:[100],failEmpty:failure});
   const nodes=visit(e.get('groups'));
   assert.ok(nodes.some(n=>n.textContent===messages.en['chart.empty']));
   assert.ok(nodes.some(n=>n.dataset.column==='oom_kills'));
   assert.equal(e.consoleErrors.length,1);assert.equal(e.get('load-errors').children.length,0);
   assert.ok(e.instances.some(chart=>!chart.destroyed && chart.data[1][0]===1));
   if(failure==='draw')assert.ok(e.instances[0].destroyed);
 }
});

test('contents links match rendered headings in order',async()=>{
 const groups=[
   {id:'nav',kind:'load',name:'load',columns:['time','load1'],units:['s','(none)'],days:['1970-01-01'],last_seen:50},
   {id:'app-test',kind:'app',name:'<img src=x>',columns:['time','count'],units:['s','count'],days:['1970-01-01'],last_seen:100}
 ];
 for(const language of ['en','ja']) {
   const e=await startViewer('extra',groups.map(group=>({group,rows:[[100,0]]})),{languages:[language]});
   const links=visit(e.get('contents-list')).filter(node=>node.tagName==='A');
   const sections=e.get('groups').children;
   assert.equal(e.get('contents').hidden,false);
   assert.equal(links.length,2);
   for(let i=0;i<links.length;i++) {
     assert.equal(links[i].href,'#'+sections[i].id);
     assert.equal(links[i].textContent,sections[i].children[0].children[0].textContent);
   }
   assert.equal(sections[0].id,'group-nav');
   assert.equal(new Set(['contents',...sections.map(node=>node.id)]).size,3);
   assert.equal(links[1].textContent,messages[language]['kind.app']+' · <img src=x>');
   assert.ok(visit(sections[0]).some(node=>node.className==='stopped'));
   e.changePeriod(3600);await settle();
   assert.equal(visit(e.get('contents-list')).filter(node=>node.tagName==='A').length,2);
   assert.equal(e.get('groups').children.length,2);
 }
 const empty=await startViewer('empty');assert.equal(empty.get('contents').hidden,true);
 const missing=await startViewer('extra',[{group:groups[0],rows:[]}],{fail:src=>src.startsWith('data/')});
 assert.equal(visit(missing.get('contents-list')).filter(node=>node.tagName==='A').length,1);
 assert.equal(missing.get('groups').children.length,1);
 assert.equal(missing.get('load-errors').children.length,1);
 const html=fs.readFileSync(require.resolve('./index.html'),'utf8');
 assert.match(html,/<nav[^>]*id="contents"[^>]*aria-labelledby="contents-title"/);
 assert.match(html,/id="contents-title"[^>]*data-i18n="page.contents"/);
});
test('last boot renders estimated time or unavailable in both languages',async()=>{
 for(const language of ['en','ja']) {
   for(const boots of [undefined,null,[],[100,null],[200,100],[0],[-1],[100,Infinity]]) {
     const e=await startViewer('empty',undefined,{languages:[language],boots});
     const last=latestBoot(boots),time=e.get('last-boot');
     const expected=last===null?messages[language]['page.unavailable']:new Intl.DateTimeFormat(language==='ja'?'ja-JP':'en-US',{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'}).format(new Date(last*1000));
     assert.equal(time.textContent,expected);
     assert.equal(time.dateTime,last===null?undefined:new Date(last*1000).toISOString());
     assert.ok(e.staticElements.some(node=>node.textContent===messages[language]['page.lastBoot']));
   }
 }
 assert.equal(messages.en['page.lastBoot'],'Last recorded boot (estimated)');
 assert.equal(messages.ja['page.lastBoot'],'記録上の最終起動(推定)');
 assert.match(fs.readFileSync(require.resolve('./index.html'),'utf8'),/<time id="last-boot"/);
});

test('directory viewer loads HD.dirs mirrors and renders safe paths, partial and signed changes',async()=>{
 const elements=await startViewer('dirs');
 const visit=node=>[node,...node.children.flatMap(visit)];
 const nodes=visit(elements.get('directories'));
 for(const text of ['Directory usage','/<script>','-1 KiB','—','0 B'])assert.ok(nodes.some(node=>node.textContent===text),text);
 assert.ok(nodes.some(node=>node.textContent.includes('partial (3 entries skipped)')));
 assert.ok(nodes.some(node=>node.textContent.includes('not in top 20 that day')));
 assert.equal(elements.get('directories').hidden,false);
 assert.equal(elements.get('load-errors').children.length,0);
});
test('directory load failures remain visible and old indexes hide the section',async()=>{
 const failed=await startViewer('dirs-missing');
 assert.ok(failed.get('load-errors').children.some(node=>node.textContent.startsWith('Directory usage unavailable:')));
 const old=await startViewer('empty');assert.equal(old.get('directories').hidden,true);
});

test('one directory day has no comparison date or zero change',async()=>{
 const elements=await startViewer('dirs-single');
 const visit=node=>[node,...node.children.flatMap(visit)];
 const nodes=visit(elements.get('directories'));
 assert.ok(nodes.some(node=>node.textContent==='no earlier day to compare'));
 assert.ok(!nodes.some(node=>node.textContent.includes('compared with')));
 const rows=nodes.filter(node=>node.scope==='row').map(node=>nodes.find(parent=>parent.children[0]===node));
 assert.equal(rows.length,2);
 for(const row of rows)assert.deepEqual(row.children.slice(-2).map(node=>node.textContent),['—','—']);
 assert.equal(elements.get('load-errors').children.length,0);
});

test('comparison and latest partial status never prints undefined counts',async()=>{
 const elements=await startViewer('dirs-missing-errors');
 const visit=node=>[node,...node.children.flatMap(visit)];
 const texts=visit(elements.get('directories')).map(node=>node.textContent);
 assert.ok(texts.some(text=>text.includes('comparison partial')));
 assert.ok(!texts.some(text=>text.includes('undefined entries skipped')));
});

test('viewer draws PSI and separated material/ratio panels with explanations',async()=>{
 const psi={id:'psi',kind:'psi',name:'psi',columns:['time','cpu_some'],units:['s','%'],days:['1970-01-01'],last_seen:100};
 const tcp={id:'tcp',kind:'tcp',name:'tcp',columns:['time','established','out_segs','retrans_segs'],units:['s','count','count/s','count/s'],days:['1970-01-01'],last_seen:100};
 const elements=await startViewer('extra',[{group:psi,rows:[[100,2]]},{group:diskGroup,rows:[[100,10,20,2,4,30,6,20]]},{group:tcp,rows:[[100,1,10,2]]}]);
 assert.equal(elements.get('load-errors').children.length,0);
 assert.equal(elements.plotted.length,8); // PSI + 4 disk charts + 3 TCP charts
 const visit=node=>[node,...node.children.flatMap(visit)];
 const nodes=visit(elements.get('groups'));
 assert.ok(nodes.some(node=>node.textContent==='Pressure stall information'));
 assert.ok(nodes.some(node=>node.textContent.includes('Not application response time')));
 assert.ok(nodes.some(node=>node.textContent.includes('not packet loss rate')));
 for(const options of elements.plotted) assert.ok(options.axes.length<=3);
 assert.ok(nodes.some(node=>node.className==='latest' && node.textContent==='3 ms'));
 assert.ok(nodes.some(node=>node.className==='latest' && node.textContent==='20 %'));
});
for (const failure of ['index','uplot','data']) test('viewer routes '+failure+' failure to its own message',async()=>{
 const elements=await startViewer(failure);
 const messages=elements.get('load-errors').children.map(item=>item.textContent);
 const expected=failure==='data'?loadError('data','data/1m/1970-01-01/cpu.js'):loadError(failure);
 assert.deepEqual(messages,[expected]);
 if(failure!=='index') assert.equal(elements.get('collected').dateTime,new Date(100000).toISOString());
});

test('load viewer omits unitless labels from legend, table and axis',async()=>{
 const elements=await startViewer('load');
 assert.equal(elements.plotted.length,1);
 const options=elements.plotted[0];
 assert.equal(options.series[1].label,'load1');
 assert.equal(options.series[2].label,'load5');
 assert.equal(options.axes[1].label,'');
 assert.equal(options.axes[1].values(null,[1])[0],'1');
 const visit=node=>[node,...node.children.flatMap(visit)];
 const latest=visit(elements.get('groups')).filter(node=>node.className==='latest');
 assert.equal(latest[0].textContent,'1');
 assert.equal(latest[3].textContent,'4');
});

test('coarse viewer excludes today; point labels use the UTC day and ticks keep the end boundary',async()=>{
 const elements=await startViewer('coarse');
 assert.equal(elements.plotted.length,1);
 assert.deepEqual(Array.from(elements.plottedData[0][0]),[1793404800]);
 assert.equal(elements.plotted[0].legend.show,false);
 const nodes=visit(elements.get('groups'));
 assert.ok(nodes.some(node=>node.textContent==='Last point in this period: 2026-10-30 (UTC day)'));
 assert.ok(nodes.some(node=>node.textContent===coarseNotice));
 assert.ok(nodes.some(node=>node.textContent==='Latest (mean)'));
 assert.deepEqual(Array.from(elements.plotted[0].axes[0].values(null,[1793404800])),['2026-10-31']);
 assert.equal(elements.plotted[0].tzDate(1793404800).getHours(),0);
 assert.deepEqual(nodes.filter(node=>node.className==='latest').map(node=>node.textContent),['1','2','3','4']);
});

test('5m viewer loads month files, hides legends and draws thin maxima only for line panels',async()=>{
 const elements=await startViewer('coarse5m');
 assert.equal(elements.plotted.length,1);
 assert.deepEqual(Array.from(elements.plottedData[0][0]),[1793318700]);
 const options=elements.plotted[0];assert.equal(options.legend.show,false);
 assert.ok(options.series.slice(5).every(series=>series.width===1 && series.fill===null));
 const formats=options.axes[0].values;
 assert.ok(formats.some(row=>row[1]==='{YYYY}'));assert.ok(JSON.stringify(formats).includes('{HH}:{mm}'));
 assert.ok(!JSON.stringify(formats).match(/\{(?:h|aa|MMM|WWW)\}/));
 assert.ok(visit(elements.get('groups')).some(node=>node.textContent==='Latest (mean)'));
});

const fixtureGroup=kind=>({...viewGroup(kind),id:kind,name:kind,days:['1970-01-01'],last_seen:100});
const statsRows=elements=>visit(elements.get('groups')).filter(node=>node.className==='stats-row');
test('B stack renderer reverses cumulative series, keeps raw table values and table-only rows colorless',async()=>{
 const g={...fixtureGroup('cpu'),columns:['time','user','system','idle'],units:['s','%','%','%']};
 const elements=await startViewer('extra',[{group:g,rows:[[40,10,20,70],[100,30,5,65]]}]);
 assert.equal(elements.get('load-errors').children.length,0);
 assert.deepEqual(JSON.parse(JSON.stringify(elements.plottedData[0])),[[40,100],[30,35],[10,30]]);
 const options=elements.plotted[0];assert.equal(options.series[1].label,'system');assert.equal(options.series[2].label,'user');
 for(const s of options.series.slice(1)) {assert.equal(s.paths.stepped.align,-1);assert.match(s.fill,/^#[0-9a-f]{6}$/);assert.equal(s.spanGaps,false);}
 assert.deepEqual(Array.from(options.scales.y.range()),[0,100]);
 assert.equal(options.legend.show,false);assert.equal(options.cursor.points.show,false);
 const rows=statsRows(elements);assert.deepEqual(rows.map(r=>r.dataset.column),['user','system','idle']);
 assert.deepEqual(rows[0].children.slice(1).map(c=>c.textContent),['10 %','20 %','30 %','30 %','—']);
 assert.equal(visit(rows[2]).filter(n=>n.className==='swatch').length,0);
 const u=elements.instances[0];u.cursor.left=50;u.cursor.idx=0;options.hooks.setCursor[0](u);
 assert.equal(rows[0].children.at(-1).textContent,'30 %'); // containing bar, not nearest point
 u.cursor.left=-10;options.hooks.setCursor[0](u);assert.equal(rows[0].children.at(-1).textContent,'—');
});
test('B mirrored renderer uses a symmetric range and positive table and axis values, even without rx',async()=>{
 assert.deepEqual(mirrorRange(0,0),[-1,1]);assert.deepEqual(mirrorRange(null,null),[-1,1]);
 const g={...fixtureGroup('net'),columns:['time','tx'],units:['s','bytes/s']};
 const elements=await startViewer('extra',[{group:g,rows:[[40,2],[100,4]]}]);
 assert.deepEqual(Array.from(elements.plottedData[0][1]),[-16,-32]);
 const options=elements.plotted[0];assert.deepEqual(Array.from(options.scales.y.range(null,-32,16)),[-32,32]);
 assert.equal(options.axes[1].values(null,[-32])[0],'32 bit/s');
 assert.equal(statsRows(elements)[0].children[4].textContent,'32 bit/s');
 assert.equal(statsRows(elements)[0].children.at(-1).textContent,'2.88 kbit');
});
test('B missing midpoint and outside cursor never show a fabricated zero',async()=>{
 const g={...fixtureGroup('net'),columns:['time','rx','tx'],units:['s','bytes/s','bytes/s']};
 const elements=await startViewer('extra',[{group:g,rows:[[40,2,3],[280,4,5]]}],{generatedAt:280});
 const options=elements.plotted[0],u=elements.instances[0],rows=statsRows(elements);
 assert.deepEqual(Array.from(elements.plottedData[0][0]),[40,160,280]);
 for(const x of [160,0,281]) {u.cursor.left=x;options.hooks.setCursor[0](u);for(const row of rows)assert.equal(row.children[5].textContent,'—');}
 u.cursor.left=200;options.hooks.setCursor[0](u);assert.equal(rows[0].children[5].textContent,'32 bit/s');
});
test('B zero observations and all-null groups render empty charts and missing tables; one observation retains its statistics',async()=>{
 for(const rows of [[],[[100,null,null]]]) {
   const g={...fixtureGroup('net'),columns:['time','rx','tx'],units:['s','bytes/s','bytes/s']};
   const elements=await startViewer('extra',[{group:g,rows}]);
   assert.equal(elements.plotted.length,0);assert.equal(elements.get('load-errors').children.length,0);
   assert.ok(visit(elements.get('groups')).some(n=>n.textContent===messages.en['chart.empty']));
   for(const row of statsRows(elements)) assert.ok(row.children.slice(1).every(c=>c.textContent==='—'));
 }
 const g={...fixtureGroup('mem'),columns:['time','used'],units:['s','bytes']};
 const one=await startViewer('extra',[{group:g,rows:[[100,1024]]}]);
 assert.equal(one.get('load-errors').children.length,0);
 assert.deepEqual(statsRows(one)[0].children.slice(1).map(c=>c.textContent),['1 KiB','1 KiB','1 KiB','1 KiB','—']);
 const zero=await startViewer('extra',[{group:{...fixtureGroup('net'),columns:['time','rx','tx'],units:['s','bytes/s','bytes/s']},rows:[[40,0,0],[100,0,0]]}]);
 assert.deepEqual(Array.from(zero.plotted[0].scales.y.range(null,0,0)),[-1,1]);
});
test('B coarse table uses samples and maxima while drawing interval means, and x never exceeds generation',async()=>{
 const g={...fixtureGroup('net'),columns:['time','rx','tx'],units:['s','bytes/s','bytes/s'],days:[],coarse:{'5m':['1970-01']}};
 const h={schema:1,group:'net',columns:['time','samples','last_time','rx','rx_max','tx','tx_max'],units:['s','count','s','bytes/s','bytes/s','bytes/s','bytes/s'],step:300};
 const e=await startViewer('extra',[{group:g,header:h,rows:[[0,2,60,1,8,2,9],[300,3,420,3,5,4,7],[86399,1,86399,5,9,5,9]]}],{period:'2678400',generatedAt:86500});
 assert.ok(e.plottedData[0][0].every(x=>x<=86500));
 assert.deepEqual(Array.from(e.plottedData[0][0]),[300,600]);
 assert.equal(e.plotted[0].series.length,3); // mirror has no maximum lines
 assert.deepEqual(statsRows(e)[0].children.slice(1).map(c=>c.textContent),['8 bit/s','17.6 bit/s','64 bit/s','24 bit/s','—','5.28 kbit']);
});
test('B unknown kind/column and missing memory column remain readable; disk usage uses one latest row',async()=>{
 const custom={...fixtureGroup('custom'),columns:['time','future'],units:['s','count']};
 const mem={...fixtureGroup('mem'),columns:['time','used','total'],units:['s','bytes','bytes']};
 const disk=fixtureGroup('disk');
 const e=await startViewer('extra',[{group:custom,rows:[[100,7]]},{group:mem,rows:[[100,20,100]]},{group:disk,rows:[[40,20,80,2,10],[100,null,80,3,10]]}]);
 assert.equal(e.get('load-errors').children.length,0);
 assert.ok(visit(e.get('groups')).some(n=>n.textContent==='future'));
 assert.ok(visit(e.get('groups')).some(n=>n.textContent==='Usage: — · Inode usage: 30 %'));
 assert.deepEqual(statsRows(e).filter(n=>['used','available'].includes(n.dataset.column))[0].children.slice(1).map(c=>c.textContent),['20 B','20 B','20 B','20 B','—']);
});
test('B Japanese and English VM screens translate every fixed label including directories and errors',async()=>{
 for(const language of ['en','ja']) {
   const e=await startViewer('dirs',undefined,{languages:[language]});
   assert.equal(e.sandbox.document.documentElement.lang,language);
   for(const element of e.staticElements) assert.equal(element.textContent,messages[language][element.dataset.i18n]);
   const texts=visit(e.get('directories')).map(n=>n.textContent);
   for(const key of ['dirs.title','dirs.path','dirs.depth','dirs.change','dirs.absent']) assert.ok(texts.includes(messages[language][key]),key);
   const g=fixtureGroup('diskio');const charts=await startViewer('extra',[{group:g,rows:[[100,10,20,2,4,30,6,20]]}],{languages:[language]});
   const nodes=visit(charts.get('groups'));
   for(const key of ['kind.diskio','panel.diskio','panel.operations','panel.busy','panel.completion','chart.columns','description.read_completion','description.write_completion','stats.min','stats.avg','stats.max','stats.last','stats.cursor']) assert.ok(nodes.some(n=>n.textContent===messages[language][key]),key);
   if(language==='ja') for(const n of nodes) assert.ok(!/No observations|Latest|Description|Series|Min|Average|Cursor|Period total|Coarse charts|Not application/.test(n.textContent),n.textContent);
   for(const failure of ['index','uplot','data','dirs-missing']) {
     const errors=await startViewer(failure,undefined,{languages:[language]});
     const key=failure==='dirs-missing'?'error.dirs':'error.'+failure;
     assert.ok(errors.get('load-errors').children[0].textContent.startsWith(messages[language][key].split('{')[0]));
   }
 }
});
test('B two quick period changes ignore a stale render and its deferred data failure',async()=>{
 const g={...fixtureGroup('load'),days:['1970-01-02'],coarse:{'5m':['1970-01']}};
 const e=await startViewer('extra',[{group:g,rows:[[86450,1,2,3,4]]}],{generatedAt:86500,hold:src=>src.startsWith('data/5m/'),fail:src=>src.startsWith('data/5m/')});
 e.changePeriod(2678400);await settle();assert.equal(e.deferred.length,1);
 e.changePeriod(3600);await settle();
 const current=e.instances.at(-1);assert.equal(e.plotted.length,2);
 e.deferred[0]();await settle();
 assert.equal(e.plotted.length,2);assert.equal(e.instances.at(-1),current);assert.ok(!current.destroyed);
 assert.equal(e.get('load-errors').children.length,0);
});
test('B explicit zoom bounds are retained while table statistics keep the selected period',async()=>{
 const e=await startViewer('load');
 const scale=e.plotted[0].scales.x;
 assert.equal(scale.min,100-86400);assert.equal(scale.max,100);
 assert.deepEqual(Array.from(scale.range(null,40,80)),[40,80]);
 assert.deepEqual(Array.from(scale.range(null,null,null)),[100-86400,100]);
 assert.equal(statsRows(e)[0].children[4].textContent,'1');
});
test('B unavailable reasons and known column names are translated; unknown data labels are retained',async()=>{
 const e=await startViewer('empty',undefined,{languages:['ja'],unavailable:[{group:'mem',column:'available',reason:'unsupported'},{group:'future-kind',column:'future-column',reason:'future-reason'}]});
 const items=e.get('unavailable-list').children.map(n=>n.textContent);
 assert.equal(items[0],'メモリ / 利用可能: 環境にありません');
 assert.equal(items[1],'future-kind / future-column: future-reason');
});
test('B invalid coarse sample counts suppress every displayed value, and labels retain the actual header step',async()=>{
 const g={...fixtureGroup('net'),columns:['time','rx'],units:['s','bytes/s'],days:[],coarse:{'5m':['1970-01']}};
 const header={schema:1,group:'net',step:240,columns:['time','samples','last_time','rx','rx_max'],units:['s','count','s','bytes/s','bytes/s']};
 const rows=[[0,0,0,9,99],[240,2,300,2,4],[480,-1,480,88,999]];
 const e=await startViewer('extra',[{group:g,header,rows}],{period:'2678400',generatedAt:86500});
 const row=statsRows(e)[0];assert.deepEqual(row.children.slice(1).map(c=>c.textContent),['16 bit/s','16 bit/s','32 bit/s','16 bit/s','—','1.92 kbit']);
 assert.deepEqual(Array.from(e.plottedData[0][1]),[null,16,null]);
 const u=e.instances[0];u.cursor.left=400;e.plotted[0].hooks.setCursor[0](u);
 assert.ok(visit(e.get('groups')).some(n=>n.textContent==='Cursor: '+formatTime(240,false)));
 u.cursor.left=600;e.plotted[0].hooks.setCursor[0](u);assert.equal(row.children[5].textContent,'—');
 const disk={...fixtureGroup('disk'),units:['s','count','bytes','count','bytes']};
 const bad=await startViewer('extra',[{group:disk,rows:[[100,2,8,3,10]]}]);
 assert.ok(visit(bad.get('groups')).some(n=>n.textContent==='Usage: — · Inode usage: —'));
});

test('FIX-B zero-based range covers missing, zero and positive values and cumulative stack maxima',()=>{
 for(const max of [null,undefined,NaN,Infinity,0,-1]) assert.deepEqual(zeroRange(max),[0,1]);
 assert.deepEqual(zeroRange(80),[0,84]);
 const data=stackSeries([[30,31,32],[70,69,68]]);
 assert.deepEqual(zeroRange(Math.max(...data.flat())),[0,105]);
});
test('FIX-B stack and line ranges start at zero; CPU and mirror keep their explicit ranges',async()=>{
 const groups=['mem','load','net','cpu'].map(kind=>({group:fixtureGroup(kind),rows:[[100,...viewHeaders[kind][0].map(()=>10)]]}));
 const e=await startViewer('extra',groups);
 assert.equal(e.get('load-errors').children.length,0);
 const [mem,oom,load,net,errors,cpu]=e.plotted;
 for(const options of [mem,oom,load,errors]) {
   assert.deepEqual(Array.from(options.scales.y.range(null,null,null)),[0,1]);
   assert.deepEqual(Array.from(options.scales.y.range(null,30,100)),[0,105]);
 }
 assert.ok(errors.series.slice(1).every(s=>s.fill===null));
 assert.deepEqual(Array.from(cpu.scales.y.range(null,30,100)),[0,100]);
 assert.deepEqual(Array.from(net.scales.y.range(null,-30,100)),[-100,100]);
});
test('FIX-B point times use four-digit years and 24-hour midnight in English and Japanese',async()=>{
 const date=new Date(2026,10,3,0,4,5),time=+date/1000;
 const options={year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'};
 assert.equal(formatTime(time,false),new Intl.DateTimeFormat('en-US',options).format(date));
 for(const language of ['en','ja']) {
   const g={...fixtureGroup('load'),days:[date.toISOString().slice(0,10)],last_seen:time};
   const e=await startViewer('extra',[{group:g,rows:[[time-60,1,2,3,4],[time,1,2,3,4]]}],{languages:[language],generatedAt:time});
   const expected=new Intl.DateTimeFormat(language==='ja'?'ja-JP':'en-US',options).format(date);
   assert.equal(e.get('collected').textContent,expected);
   assert.match(expected,/2026/);assert.match(expected,/00:04:05/);assert.doesNotMatch(expected,/AM|PM/);
   const nodes=visit(e.get('groups'));
   assert.ok(nodes.some(n=>n.textContent===t(language,'chart.lastPoint',{time:expected})));
   const u=e.instances[0];u.cursor.left=time;e.plotted[0].hooks.setCursor[0](u);
   assert.ok(nodes.some(n=>n.textContent===t(language,'stats.cursor')+': '+expected));
 }
 assert.equal(formatTime(time,true),date.toISOString().slice(0,10)+' (UTC day)');
});
test('FIX-B coarse notes appear once per group, after the last panel',async()=>{
 for(const language of ['en','ja']) {
   const g={...fixtureGroup('net'),days:[],coarse:{'5m':['1970-01']}};
   const h={schema:1,group:'net',step:300,columns:['time','samples','last_time',...g.columns.slice(1).flatMap(c=>[c,c+'_max'])],units:['s','count','s',...g.units.slice(1).flatMap(u=>[u,u])]};
   const row=[0,5,240,...g.columns.slice(1).flatMap(()=>[2,4])];
   const e=await startViewer('extra',[{group:g,header:h,rows:[row]}],{languages:[language],generatedAt:86500,period:'2678400'});
   assert.equal(e.plotted.length,2);
   const section=e.get('groups').children[0],nodes=visit(section);
   for(const key of ['note.statsCoarse','note.coarse']) {
     const notes=nodes.filter(n=>n.textContent===messages[language][key]);assert.equal(notes.length,1,key);
     const lastTable=section.children.map(n=>n.className).lastIndexOf('stats-wrap');
     assert.ok(section.children.indexOf(notes[0])>lastTable);
   }
 }
});

test('FIX-FINAL double-click restores the selected window, preserving the unobserved part of today',async()=>{
 for(const fixture of ['load','coarse']) {
   const e=await startViewer(fixture),options=e.plotted[0];
   let range={min:40,max:80},defaultCalled=false;
   const u={setScale(scale,next){assert.equal(scale,'x');range=next;}};
   const dblclick=options.cursor.bind.dblclick(u,{},()=>{defaultCalled=true;});
   dblclick({preventDefault(){}});
   assert.equal(defaultCalled,false);
   assert.deepEqual({min:range.min,max:range.max},{min:options.scales.x.min,max:options.scales.x.max});
   if(fixture==='coarse') {
     const observedEnd=e.plottedData[0][0].at(-1);
     assert.ok(range.max>observedEnd);
     assert.equal(range.max,Date.parse('2026-10-31T12:00:00Z')/1000);
   }
 }
});
test('FIX-FINAL monthly ticks include both month and day with the year at a boundary',async()=>{
 const e=await startViewer('coarse5m');
 const month=e.plotted[0].axes[0].values.find(row=>row[0]===2419200);
 assert.equal(month[1],'{M}/{D}');assert.equal(month[2],'\n{YYYY}');
});
