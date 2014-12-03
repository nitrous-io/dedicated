App.IdePartProcessingView = Em.View.extend
  templateName: 'templates/box/ide/part-processing'
  elementId: 'ide_parts_processing_view'

  content: null

  title: (->
    return unless content = @get('content')
    "#{content.get('humanized')?.name} #{content.get('version')}"
  ).property 'content.humanized', 'content.version'

  scrollOutputToBottom: ->
    @$('.autoparts-output')?.scrollTop this._outputScrollHeight()

  _outputScrollHeight: ->
    @$('.autoparts-output')?.get(0).scrollHeight

  _scrollOutputWindowToBottom: (->
    if this._outputScrollHeight() > @$('.autoparts-output')?.height()
      Em.run.scheduleOnce 'afterRender', this, 'scrollOutputToBottom'
  ).observes 'controller.partsSocket.outputStream.content.length'
