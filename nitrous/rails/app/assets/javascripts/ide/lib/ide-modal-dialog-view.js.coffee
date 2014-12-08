App.IdeModalDialogView = Em.View.extend
  classNames: ['modal', 'hide', 'fade']
  attributeBindings: ['data-backdrop', 'data-keyboard', 'data-show', 'tabindex']

  init: ->
    @set 'data-backdrop', 'static'
    @set 'data-keyboard', 'true'
    @set 'data-show', 'false'
    @set 'tabindex', -1
    @_super.apply this, arguments

  didInsertElement: ->
    @$().modal().on 'hide', =>
      this.onHide()

  showOrHide: (->
    if @get 'content'
      this.show()
    else
      this.hide()
  ).observes 'content'

  show: ->
    @$().modal 'show'
    window.setTimeout =>
      this.viewDidAppear()
    , 500

  hide: ->
    @$().modal 'hide'

  onHide: ->
    @set 'content', null
    window.setTimeout =>
      this.viewDidDisappear()
    , 500

  viewDidAppear: ->
    selector = @get('focusSelector')
    return if !selector

    # Workaround for Chrome 35 where the focus switches
    # to document.body after the focus() call below is
    # run.  Restore focus to text field on blur only
    # _once_ to handle this situation.
    @$(selector).one 'blur', =>
      if document.activeElement == document.body
        @$(selector).focus()

    @$(selector).focus()

  viewDidDisappear: ->

  didClickCancel: ->
    this.hide()

