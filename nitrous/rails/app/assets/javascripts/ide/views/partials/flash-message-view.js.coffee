App.FlashMessageView = Em.View.extend
  templateName: 'templates/partials/flash-message'
  elementId: 'flash_messages'
  messageBinding: 'App.flashMessage'

  showOrHide: (->
    if @get 'message'
      this.show()
    else
      this.hide()
  ).observes 'message'

  show: ->
    @$('.alert').removeClass('in').addClass('in')

  hide: ->
    @$('.alert').removeClass('in')

  didClickClose: ->
    return if @get('message.sticky')
    @set 'message', null
    false
