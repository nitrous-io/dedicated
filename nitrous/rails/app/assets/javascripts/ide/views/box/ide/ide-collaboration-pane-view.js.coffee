App.IdeCollaborationPaneView = Em.View.extend
  templateName: 'templates/box/ide/collaboration-pane'
  elementId: 'ide_collaboration_pane_view'
  chatFieldText: null
  hiddenBinding: 'App.router.ideLayoutController.collaborationPaneHidden'
  classNameBindings: ['hidden']

  didClickHideChat: ->
    @set 'hidden', true

  didClickSend: ->
    if message = @get 'chatFieldText'
      @get('controller').sendMessage message, App.CollabMessage.TypeChat
      @set 'chatFieldText', null

  scrollChatToBottom: ->
    @$('.chat')?.scrollTop this._chatMaxScroll()

  _hiddenDidChange: (->
    Em.run.scheduleOnce 'afterRender', this, 'scrollChatToBottom'
  ).observes 'hidden'

  _chatMaxScroll: ->
    $chat = @$('.chat')
    $messages = $('ul', $chat)
    if $messages && $chat
      Math.max 0, ($messages.height() - $chat.height())

  _scrollChatToBottomIfAlreadyAtBottom: (->
    $chat = @$('.chat')
    if ($chat.length > 0) && ($chat.scrollTop() == this._chatMaxScroll())
      Em.run.scheduleOnce 'afterRender', this, 'scrollChatToBottom'
  ).observes 'controller.messages.length'

  _consoleHeightDidChange: (->
    consoleHeight = App.get 'router.ideLayoutController.consoleHeight'
    @$().css 'paddingBottom', (consoleHeight + 4)
  ).observes 'App.router.ideLayoutController.consoleHeight'

App.IdeCollaborationPaneOnlineUsersButton = Em.View.extend
  tagName: 'button'
  classNames: ['btn', 'ide_collaboration_pane_online_users_button']
  attributeBindings: ['title']

  mouseEnter: ->
    $('#collaboration_pane .popover').show()

  mouseLeave: ->
    $('#collaboration_pane .popover').fadeOut(100)

  click: ->
    @get('controller').showManageCollaboratorsDialog()

App.IdeCollaborationPaneTextField = Em.TextField.extend
  insertNewline: ->
    @get('parentView')?.didClickSend()

App.IdeCollaborationPaneOnlineUserColorView = Em.View.extend
  tagName: 'span'
  classNames: ['user-color']
  color: null
  username: null

  didInsertElement: ->
    @_super()
    if @get 'color'
      this.setElementColor()
    else if @get('username')
      this.setColorFromUsername()

  setElementColor: (->
    if color = @get 'color'
      @$().css 'backgroundColor', color
  ).observes 'color'

  setColorFromUsername: ->
    username = @get 'username'
    users = @get('controller').get 'users'
    matchedUser = users.find (user) ->
      user.get('name') == username

    if matchedUser
      @set 'color', matchedUser.get('color')
    else
      # set a default color
      @set 'color', '#666666'
