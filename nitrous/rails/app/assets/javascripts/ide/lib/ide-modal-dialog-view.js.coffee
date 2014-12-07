App.IdeModalDialogView = Em.View.extend
  classNames: ['ui', 'modal']

  showOrHide: (->
    if @get 'content'
      @show()
    else
      @hide()
  ).observes 'content'

  show: ->
    ui.showModal(@$())
    Em.run.scheduleOnce 'afterRender', this, ->
      @viewDidAppear()

  hide: ->
    ui.hideModal(@$())
    Em.run.scheduleOnce 'afterRender', this, ->
      @viewDidDisappear()

  viewDidAppear: ->
    selector = @get('focusSelector')
    return if !selector
    @$(selector).focus()

  viewDidDisappear: ->

  didClickCancel: ->
    @hide()
