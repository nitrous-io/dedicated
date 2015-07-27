App.HostStats = Em.Object.extend
  memoryTotal: null
  memoryUsage: null
  numCPU: null
  perCPUUsage: null
  storageTotal: null
  storageUsage: null
  uptime: null

  memory_total: Em.computed.alias 'memoryTotal'
  memory_usage: Em.computed.alias 'memoryUsage'
  num_cpu: Em.computed.alias 'numCPU'
  per_cpu_usage: Em.computed.alias 'perCPUUsage'
  storage_total: Em.computed.alias 'storageTotal'
  storage_usage: Em.computed.alias 'storageUsage'
