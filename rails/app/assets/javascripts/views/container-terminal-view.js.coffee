App.ContainerTerminalView = Em.View.extend
  elementId: 'container-terminal-view'
  fullscreen: false
  classNameBindings: ['fullscreen']

  didInsertElement: ->
    @_super()
    $('html').addClass('fullscreen')
    $('<div>').addClass('container-terminal-placeholder').css('flex-grow': 1).hide().insertAfter(@$())

  willDestroyElement: ->
    @_super()
    $('html').removeClass('fullscreen')
    @$().nextAll('.container-terminal-placeholder').remove()
    @set 'fullscreen', false

  actions:
    toggleFullscreen: ->
      # animate fullscreen transition
      $this = @$()
      $placeholder = @$().nextAll('.container-terminal-placeholder')
      $button = @$('.fullscreen-btn button')
      $button.attr('disabled', true).css(cursor: 'default')

      if @get('fullscreen')
        offset = $placeholder.offset()
        width = $placeholder.width()
        height = $placeholder.height()
      else
        offset = $this.offset()
        width = $this.width()
        height = $this.height()

      $this.css(top: offset.top, left: offset.left, width: width, height: height)
      $this.css(position: 'fixed', 'z-index': 1, 'box-shadow': '0 1px 4px rgba(0, 0, 0, .4)')

      if @get('fullscreen')
        $placeholder.hide()
      else
        $placeholder.show()

      window.requestAnimationFrame =>
        $this.addClass('animate')
        @toggleProperty 'fullscreen'
        window.setTimeout(->
          $this.css(top: '', left: '', width: '', height: '', position: '', 'z-index': '', 'box-shadow': '')
          $button.attr('disabled', false).css(cursor: '')
          $this.removeClass('animate')
        , 601)
        $button.blur()
