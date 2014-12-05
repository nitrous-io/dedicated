App.FileFindCommand = klass = Em.Object.extend
  dialog: null

  bindEvents: (socket) ->
    socket.on 'filefind.commandOutput', (data) =>
      this.onOutput(data)
    socket.on 'filefind.commandError', (data) =>
      this.onError(data)
    socket.on 'filefind.commandComplete', (data) =>
      this.onComplete(data)

  send: (socket, dialog, pattern, path, callback) ->
    @set 'dialog', dialog
    socket.sendAs 'filefind', 'filefind', pattern, path, =>
      callback(this, klass.keyFor(pattern, path))

  onOutput: (data) ->
    @get('dialog')?.addResults(klass.parseData(data)...)

  onError: (data) ->
    [..., errorSlug] = klass.parseData(data)
    @get('dialog')?.set 'error', errorSlug

  onComplete: (data) ->
    [pattern, path, ...] = klass.parseData(data)
    @get('dialog')?.searchComplete(pattern, path)

klass.reopenClass
  # separator between search key and data
  dataDelimiter: "!!\n"

  # separator between file pattern and path so that (pattern1, path1) and
  # (pattern2, path2) do not overlap
  searchKeyDelimiter: "\\\\"

  keyFor: (pattern, path) ->
    # pattern cannot have dots as it will cause issues when setting
    # "results.#{key}"
    [pattern, path].join(this.searchKeyDelimiter).replace(/\./g, "~")

  parseData: (data) ->
    [key, results...] = data.split(klass.dataDelimiter)
    results = results.join('')
    [pattern, path...] = key.split(klass.searchKeyDelimiter)
    path = path.join('')
    [pattern, path, results]
