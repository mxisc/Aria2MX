export type TrackerSubscriptionEntry = {
  key: string
  name: string
  summary: string
  url: string
  homepage: string
  tags: string[]
  note: string
}

export const builtInTrackerSubscriptions: TrackerSubscriptionEntry[] = [
  {
    key: 'ngosang-best',
    name: 'ngosang / trackers_best',
    summary: '偏精简的一组公开 BT Tracker，适合作为默认订阅源。',
    url: 'https://raw.githubusercontent.com/ngosang/trackerslist/master/trackers_best.txt',
    homepage: 'https://github.com/ngosang/trackerslist',
    tags: ['GitHub', '精简', '推荐'],
    note: '列表更短，通常更适合直接作为 aria2 的 bt-tracker 默认值。',
  },
  {
    key: 'ngosang-all',
    name: 'ngosang / trackers_all',
    summary: '更完整的一组公开 BT Tracker，数量更多，覆盖更广。',
    url: 'https://raw.githubusercontent.com/ngosang/trackerslist/master/trackers_all.txt',
    homepage: 'https://github.com/ngosang/trackerslist',
    tags: ['GitHub', '完整', '高覆盖'],
    note: '会写入更多 tracker，适合希望尽量扩大可发现节点范围的场景。',
  },
  {
    key: 'newtrackon-stable',
    name: 'newtrackon / stable',
    summary: '公开的稳定版 Tracker API，返回纯文本 tracker 列表。',
    url: 'https://newtrackon.com/api/stable',
    homepage: 'https://newtrackon.com/',
    tags: ['API', '稳定', '纯文本'],
    note: '接口直接返回纯文本列表，适合后端按行抓取后写入 aria2。',
  },
  {
    key: 'adysec-best',
    name: 'adysec / trackers_best',
    summary: '聚合多个公开源并做可用性筛选的优选 Tracker 列表。',
    url: 'https://raw.githubusercontent.com/adysec/tracker/main/trackers_best.txt',
    homepage: 'https://github.com/adysec/tracker',
    tags: ['GitHub', '聚合', '优选'],
    note: '适合作为增强型候选源，适合希望使用聚合筛选结果的场景。',
  },
]
