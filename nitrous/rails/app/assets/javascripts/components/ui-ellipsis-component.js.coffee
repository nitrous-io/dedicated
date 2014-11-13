App.UiEllipsisComponent = Em.Component.extend
  tagName: 'span'
  classNames: ['ui-ellipsis']
  content: null

  timer: null

  didInsertElement: ->
    @_super()
    i = 0
    @set 'timer', window.setInterval(=>
      i = (i + 1) % 4
      str = ''
      for j in [0...i]
        str += '.'
      @set 'content', str
    , 1000)

  willDestroyElement: ->
    @_super()
    if timer = @get 'timer'
      window.clearInterval(timer)
