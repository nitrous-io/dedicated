#= require ../mixins/inject-terminal-iframe-mixin

App.ContainerConsoleView = Em.View.extend App.InjectTerminalIframeMixin,
  elementId: 'container-console-view'
  classNameBindings: ['controller.fullscreen:fullscreen']
  classNames: ['console-view']

  iframeInjected: false

  didInsertElement: ->
    @_super()
    $('html').addClass('fullscreen')
    if @get('controller.ready')
      @injectIframe()

  willDestroyElement: ->
    @_super()
    @$('iframe')[0]?.contentWindow?.NitrousTerminal?.destroy()
    $('html').removeClass('fullscreen')
    @set 'controller.fullscreen', false

  injectIframe: (->
    return unless @get 'controller.ready'
    return if @get 'iframeInjected'
    @set 'iframeInjected', true
    Em.run.scheduleOnce 'afterRender', this, ->
      controller = @get 'controller'

      @injectTerminalIframe (terminal) =>
        terminal.term.write 'Connecting...'
        terminal.willOpen = =>
          controller.makeUrlAndConnect(terminal)
        terminal.open()
  ).observes 'controller.ready'

  actions:
    toggleFullscreen: ->
      # animate fullscreen transition
      $termWindow = @$('.terminal-window')
      $placeholder = @$('.terminal-placeholder')
      $button = @$('.fullscreen-btn button')
      $button.attr('disabled', true).css(cursor: 'default')

      if @get('controller.fullscreen')
        offset = $placeholder.offset()
        width = $placeholder.width()
        height = $placeholder.height()
      else
        offset = $termWindow.offset()
        width = $termWindow.width()
        height = $termWindow.height()

      $termWindow.css(top: offset.top, left: offset.left, width: width, height: height)
      $termWindow.css(position: 'fixed', 'z-index': 1, 'box-shadow': '0 1px 4px rgba(0, 0, 0, .4)')

      window.requestAnimationFrame =>
        $termWindow.addClass('animate')
        @toggleProperty 'controller.fullscreen'
        window.setTimeout(->
          $termWindow.css(top: '', left: '', width: '', height: '', position: '', 'z-index': '', 'box-shadow': '')
          $button.attr('disabled', false).css(cursor: '')
          $termWindow.removeClass('animate')
          @$('iframe')[0]?.contentWindow?.NitrousTerminal?.focus()
        , 601)
        $button.blur()
