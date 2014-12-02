App.CopyButtonView = Em.View.extend
  tagName: 'a'
  classNames: ['copy-button-view', 'btn']
  template: Em.Handlebars.compile("<i class='fa fa-cut'></i>")
  attributeBindings: ['title', 'dataClipboardText:data-clipboard-text']
  title: 'copy to clipboard'
  value: null
  dataClipboardText: null
  zeroClipboard: null

  didInsertElement: ->
    @_super()
    clip = new ZeroClipboard(@$())
    @set 'zeroClipboard', clip

    clip.on 'load', =>
      bridge = $(clip.htmlBridge)
      bridge.tooltip title: => @get('title')

    clip.on 'mouseout', =>
      @set 'title', 'copy to clipboard'

    clip.on 'complete', =>
      @set 'title', 'copied!'
      bridge = $(clip.htmlBridge)
      bridge.tooltip('show')

  willDestroyElement: ->
    @_super()
    clip = @get 'zeroClipboard'
    clip.handlers = {}
    @set 'zeroClipboard', null
