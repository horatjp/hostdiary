/* Offline viewer vocabulary. Pure lookup, independent of DOM and navigator. */
(function () {
  'use strict';
  // Keep translations together so additions cannot silently lose one language.
  const entries = {
    'page.title':['Resource history','リソース履歴'],
    'page.windowTitle':['hostdiary · Resource history','hostdiary · リソース履歴'],
    'page.collected':['Last collected','最終収集'],
    'page.lastBoot':['Last recorded boot (estimated)','記録上の最終起動(推定)'],
    'page.contents':['Contents','目次'],
    'page.unavailable':['Not available','取得できません'],
    'page.timezone':["Times shown in your browser's time zone; 2-year observations use UTC days",'時刻はブラウザのタイムゾーンで表示します。2年表示の観測は UTC 日単位です'],
    'page.history':['History','期間'],
    'page.items':['Unavailable items','取得できない項目'],
    'page.noscript':['Enable JavaScript to view resource history.','リソース履歴を表示するには JavaScript を有効にしてください。'],
    'page.footer':['Collected history, without thresholds or alerts.','収集した履歴を表示します。しきい値やアラートはありません。'],
    'page.license':['uPlot license','uPlot ライセンス'],
    'period.3600':['1 hour','1時間'], 'period.86400':['1 day','1日'],
    'period.604800':['1 week','1週間'], 'period.2678400':['1 month','1か月'],
    'period.31536000':['1 year','1年'], 'period.63072000':['2 years','2年'],
    'status.index':['Loading collection index…','収集索引を読み込み中…'],
    'status.loading':['Loading observations…','観測値を読み込み中…'],
    'status.groups':['{count} groups · gaps indicate missing observations','{count} グループ · 途切れは観測の欠測を示します'],
    'status.empty':['No collected groups yet.','まだ収集されたグループがありません。'],
    'status.stopped':['stopped (last: {time})','停止中 (最終: {time})'],
    'error.index':['Collection index unavailable. Restore index.js beside index.html.','収集索引を読み込めません。index.html と同じ場所に index.js を戻してください。'],
    'error.uplot':['Chart library unavailable. Restore assets/vendor/uplot-1.6.32/.','グラフライブラリを読み込めません。assets/vendor/uplot-1.6.32/ を戻してください。'],
    'error.data':['History file unavailable: {path}. Restore this data file.','履歴ファイルを読み込めません: {path}。このデータファイルを戻してください。'],
    'error.display':['Could not display collection history.','収集履歴を表示できませんでした。'],
    'error.schema':['The collection index is missing or uses an unsupported schema.','収集索引がないか、未対応の schema です。'],
    'error.load':['Could not load {path}','{path} を読み込めませんでした'],
    'error.readable':['No readable data in {path}','{path} に読み取れるデータがありません'],
    'error.dirs':['Directory usage unavailable: {message}','ディレクトリ使用量を読み込めません: {message}'],
    'error.directorySchema':['Invalid directory usage: {day}','ディレクトリ使用量の形式が不正です: {day}'],
    'allColumns':['all columns','全列'],
    'reason.unsupported':['unsupported','環境にありません'],
    'reason.not_found':['not found','対象がありません'],
    'reason.error':['read or parse error','読み取り・解析エラー'],
    'chart.empty':['No observations in this period.','この期間の観測値はありません。'],
    'chart.columns':['Column descriptions','列の説明'],
    'chart.column':['Column','列'], 'chart.description':['Description','説明'],
    'chart.time':['Time','時刻'], 'chart.lastPoint':['Last point in this period: {time}','この期間の最後の点: {time}'],
    'chart.utcDay':['UTC day','UTC 日'],
    'stats.series':['Series','系列'], 'stats.min':['Min','最小'], 'stats.avg':['Average','平均'],
    'stats.max':['Max','最大'], 'stats.last':['Latest','最新'], 'stats.cursor':['Cursor','カーソル位置'],
    'stats.total':['Period total','期間合計'], 'stats.period':['Min / average / max for {period}','{period}の最小・平均・最大'],
    'stats.mean':['Latest (mean)','最新 (区間平均)'],
    'stats.meanDerived':['Latest (mean; derived: ratio of means)','最新 (区間平均、派生は平均の比)'],
    'unit.count':['count','件'], 'unit.count/s':['count/s','件/s'],
    'note.coarse':['Coarse values cover completed UTC days. See 1 day for today. If recent days are missing, check 1 day / 1 week.','粗い値は完了した UTC 日を対象にします。今日の値は1日表示で確認してください。最近の日が欠けている場合は1日・1週間表示を確認してください。'],
    'note.statsCoarse':['Minimum, average and latest are interval averages; maximum is the 1-minute maximum (derived series use the maximum ratio).','最小・平均・最新は区間平均、最大は1分値の最大(派生は比の最大)'],
    'note.total':['Estimate without correction for missing rows or collection intervals other than 60 seconds.','収集間隔が60秒でない行・欠測は補正しない目安'],
    'note.oom':['Counts since the previous collection, including kills caused by cgroup memory limits.','前回の収集からの回数です。cgroup のメモリ上限による kill も含みます。'],
    'note.bootMarkers':['Dashed line: reboot (estimated)','破線: 再起動(推定)'],
    'note.oomCoarse':['Mean of per-collection counts in each interval (not a total). The faint line is the maximum. A positive maximum indicates kills even when the mean is zero. Collection intervals and missing values are not corrected. Includes kills caused by cgroup memory limits.','区間内の収集回数値の平均です(合計ではありません)。薄い線は最大。平均が0でも最大が正なら kill はありました。収集間隔や欠測は補正しません。cgroup のメモリ上限による kill も含みます。'],
    'note.disk':['Top is used + avail (excluding root reserved space).','上端は used + avail(root の予約分を含まない)'],
    'note.net':['Displayed as bit/s (stored as bytes/s).','bit/s で表示します(保存値は bytes/s)。'],
    'note.ratio':['Coarse charts divide means, approximating count-weighted values only with nearly constant intervals and matching null patterns. Rounded-away materials are hidden.','粗い値は平均同士の比です。収集間隔がほぼ一定で欠測の並びが同じ場合だけ件数加重の近似になります。丸めで消えた材料の比は表示しません。'],
    'description.read_completion':['Average read completion time = read_time / read_ops. Not application response time.','平均読み込み完了時間 = read_time / read_ops。アプリの応答時間ではありません。'],
    'description.write_completion':['Average write completion time = write_time / write_ops. Not application response time.','平均書き込み完了時間 = write_time / write_ops。アプリの応答時間ではありません。'],
    'description.retrans_ratio':['Retransmission ratio = retrans_segs / out_segs × 100; rough indicator, not packet loss rate.','再送率 = retrans_segs / out_segs × 100。目安であり、パケットロス率ではありません。'],
    'traffic.month':['This month (UTC, estimate)','今月 (UTC、目安)'],
    'traffic.last30d':['Last 30 days (estimate)','直近30日 (目安)'],
    'traffic.empty':['no data','データなし'], 'traffic.missing':['{count} days missing','{count}日欠測'],
    'traffic.missingOne':['{count} day missing','{count}日欠測'],
    'disk.usedRatio':['Usage','使用率'], 'disk.inodeRatio':['Inode usage','inode 使用率'],
    'dirs.title':['Directory usage','ディレクトリ使用量'],
    'dirs.day':['UTC day: {day}','UTC 日付: {day}'], 'dirs.compare':['compared with {day}','比較日: {day}'],
    'dirs.note':['Parent and child entries overlap; do not add them. Depth is relative to each mount point. Paths use \\xHH escapes for non-printable bytes and backslashes.','親と子の項目は重複するので合算しないでください。深さは各マウントポイントからの相対値です。パスの非表示バイトとバックスラッシュは \\xHH で表します。'],
    'dirs.noEarlier':['no earlier day to compare','比較できる過去の日がありません'],
    'dirs.absent':['—: not in top 20 that day, or no comparison filesystem available. Absence does not mean zero.','—: その日の上位20件に含まれないか、比較用のファイルシステムがありません。不在は0を意味しません。'],
    'dirs.total':['total','合計'], 'dirs.comparison':['comparison','比較対象'],
    'dirs.complete':['complete','完了'], 'dirs.partial':['partial','一部取得'],
    'dirs.skipped':['partial ({count} entries skipped)','一部取得 ({count}件をスキップ)'],
    'dirs.path':['Path','パス'], 'dirs.depth':['Depth','深さ'], 'dirs.change':['Change','増減'],
    'kind.cpu':['CPU','CPU'], 'kind.load':['Load','負荷'], 'kind.mem':['Memory','メモリ'],
    'kind.swap':['Swap','スワップ'], 'kind.procs':['Processes','プロセス'],
    'kind.tcp':['TCP connections','TCP 接続'], 'kind.net':['Network','通信量'],
    'kind.disk':['Filesystem','ファイルシステム'], 'kind.diskio':['Disk I/O','ディスク I/O'],
    'kind.app':['Application','アプリケーション'], 'kind.psi':['Pressure stall information','リソース待ち時間 (PSI)'],
    'panel.cpu':['CPU usage','CPU 使用率'], 'panel.load':['Load and CPU count','負荷と CPU 数'],
    'panel.mem':['Memory usage','メモリ使用量'], 'panel.swap':['Swap usage','スワップ使用量'],
    'panel.oom':['OOM kills','OOM kill'],
    'panel.swapio':['Swap I/O','スワップ入出力'], 'panel.procs':['Process states','プロセス状態'],
    'panel.tcp':['TCP states','TCP 状態'], 'panel.segments':['TCP segments','TCP セグメント'],
    'panel.retrans':['Retransmission ratio','再送率'], 'panel.net':['Network traffic','通信量'],
    'panel.errors':['Network errors and drops','通信エラーとドロップ'],
    'panel.disk':['Filesystem usage','ファイルシステム使用量'], 'panel.inodes':['Inodes','inode 数'],
    'panel.diskio':['Throughput','転送量'], 'panel.operations':['Operations','処理件数'],
    'panel.busy':['Utilization','使用率'], 'panel.completion':['Average completion time','平均完了時間'],
    'panel.psi':['Pressure stall information','リソース待ち時間 (PSI)'], 'panel.generic':['Observations','観測値']
  };
  const columns = {
    rx:'受信', tx:'送信', rx_errors:'受信エラー', rx_drops:'受信ドロップ', tx_errors:'送信エラー', tx_drops:'送信ドロップ',
    total:'合計', available:'利用可能', used:'使用中', buffers:'バッファ', cached:'キャッシュ', oom_kills:'OOM kill',
    in:'スワップイン', out:'スワップアウト', running:'実行中', sleeping:'待機中', disk_sleep:'ディスク待ち', stopped:'停止', zombie:'ゾンビ',
    other:'その他', tcp_other:'other', avail:'空き', inodes_used:'inode 使用', inodes_total:'inode 合計', read:'読み込み', write:'書き込み',
    read_ops:'読み込み回数', write_ops:'書き込み回数', busy:'使用率', read_time:'読み込み時間', write_time:'書き込み時間',
    load1:'1分平均', load5:'5分平均', load15:'15分平均', cpus:'CPU 数', out_segs:'送信セグメント', retrans_segs:'再送セグメント',
    swap_free:'空き', read_completion:'平均読み込み完了時間', write_completion:'平均書き込み完了時間', retrans_ratio:'再送率',
    count:'件数', rss:'常駐メモリ', started:'新規起動数'
  };
  for (const [column, ja] of Object.entries(columns)) entries['column.' + column] = [column, ja];
  entries['column.tcp_other'][0] = 'other';
  for (const [column, label] of Object.entries({swap_free:'Free', read_completion:'Read completion', write_completion:'Write completion', retrans_ratio:'Retransmission ratio'})) entries['column.' + column][0] = label;
  for (const column of ['user','nice','system','iowait','irq','softirq','steal','idle','established','syn_sent','syn_recv','fin_wait1','fin_wait2','time_wait','close','close_wait','last_ack','listen','closing','cpu_some','memory_some','memory_full','io_some','io_full']) entries['column.' + column] = [column, column];
  const messages = {en:{}, ja:{}};
  for (const [key, pair] of Object.entries(entries)) { messages.en[key] = pair[0]; messages.ja[key] = pair[1]; }
  function detectLanguage(languages) {
    const first = Array.isArray(languages) ? languages.find(value => typeof value === 'string' && value.length > 0) : undefined;
    return first?.split('-')[0].toLowerCase() === 'ja' ? 'ja' : 'en';
  }
  function t(lang, key, params = {}) {
    const dictionary = Object.hasOwn(messages, lang) ? messages[lang] : messages.en;
    const value = Object.hasOwn(dictionary, key) ? dictionary[key] : Object.hasOwn(messages.en, key) ? messages.en[key] : String(key);
    return value.replace(/\{([^{}]+)\}/g, (token, name) => params != null && Object.hasOwn(params, name) ? String(params[name]) : token);
  }
  const api = {detectLanguage, messages, t};
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else window.HDI18n = api;
}());
