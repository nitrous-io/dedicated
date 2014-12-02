App.IdeCollaborationPaneController = Em.Controller.extend
  manageCollaboratorsDialog: null
  firebaseAuthenticated: false
  firebase: null                # <host>
  firebaseUsers: null           # <host>/box/:box_id/users
  firebasePresence: null        # <host>/box/:box_id/users/:username
  firebaseChat: null            # <host>/box/:box_id/chat
  users: null                   # array of users
  messages: null                # chat messages
  userColor: null               # color of current user

  initialize: ->
    this._initFirebase()
    @set 'users', []
    @set 'messages', []

  reset: ->
    this.hideManageCollaboratorsDialog()
    this._resetFirebase()
    @set 'users', null
    @set 'messages', null
    @set 'userColor', null

  showManageCollaboratorsDialog: ->
    box = App.get 'router.boxIdeController.content'
    if box.get('ownBox')
      @set 'manageCollaboratorsDialog', App.ManageCollaboratorsDialog.create()

  hideManageCollaboratorsDialog: ->
    @set 'manageCollaboratorsDialog', null

  sendMessage: (message, messageType) ->
    message = message?.trim()
    firebaseChat = @get 'firebaseChat'
    username = App.get 'currentUser.username'
    if firebaseChat && username && messageType
      firebaseChat.push [username, message, messageType]
      if messageType == App.CollabMessage.TypeChat
        this._notifyMentionsToOfflineUsers message
      App.util.mixpanelIncrement 'ide_send_event_notification'

  _notifyMentionsToOfflineUsers: (message) ->
    if mentions = this._extractMentions(message)
      activeUsers = _.map @get('users'), (user) -> user.name
      for mention in mentions
        unless activeUsers.contains mention
          boxId = App.get 'router.boxIdeController.content.id'
          App.Collaborator.notifyMention mention, boxId

  _extractMentions: (message) ->
    if !message || !message.match(/[@＠]/)
      return []

    mentions = []
    # regex based on twitter-text-js
    # https://github.com/twitter/twitter-text-js/blob/master/twitter-text.js
    validMentionRegex = ///
      (?:^|[^a-zA-Z0-9_!\#$%&*@＠]:?)  # Preceding character
      ([@＠])                          # At mark
      ([a-zA-Z0-9_]{1,20})             # Screen name
    ///g

    message.replace validMentionRegex, (withPreceding, atSign, mention) ->
      mentions.push mention
    mentions

  _initFirebase: ->
    firebase = @get('firebase') || new Firebase App.get('config.firebaseHost')
    @set 'firebase', firebase

    boxId = App.get 'router.boxIdeController.content.id'

    $.get('/firebase/auth_token', box_id: boxId).done (data) =>
      if data.auth_token?
        firebase.auth data.auth_token, (error) =>
          unless error
            @set 'firebaseAuthenticated', true
            this._initFirebaseUsers()
            this._setUserColor(this._initUserChatPresence.bind(this))

  _resetFirebase: ->
    this._sendLeftMessage()
    this._resetFirebaseUsers()
    this._resetFirebasePresence()
    this._resetFirebaseChat()
    @set 'firebaseAuthenticated', false
    @get('firebase')?.unauth()

  _initFirebaseUsers: ->
    boxId = App.get 'router.boxIdeController.content.id'
    firebaseUsersURL = "#{App.get 'config.firebaseHost'}/box/#{boxId}/users"
    firebaseUsers = new Firebase firebaseUsersURL
    firebaseUsers.on 'child_added', (userObj) =>
      color = userObj.child('color').val()
      name = userObj.child('name').val()
      @get('users')?.addObject App.CollabSessionUser.create name: name, color: color
    firebaseUsers.on 'child_removed', (userObj) =>
      name = userObj.child('name').val()
      if users = @get('users')
        removed_user = users.find (user) ->
          user.get('name') == name
        users.removeObject removed_user
    @set 'firebaseUsers', firebaseUsers

  _resetFirebaseUsers: ->
    @get('firebaseUsers')?.off()
    @set 'firebaseUsers', null

  _setUserColor: (callback) ->
    boxId = App.get 'router.boxIdeController.content.id'
    firebaseUserColorIdURL = "#{App.get 'config.firebaseHost'}/box/#{boxId}/next_user_color_id"
    firebaseUserColorId = new Firebase firebaseUserColorIdURL
    firebaseUserColorId.once 'value', (userColorIdSnapshot) =>
      userColorId = (userColorIdSnapshot.val() ? 0)
      color = App.UserColors.nextColor(userColorId)
      @set 'userColor', color
      firebaseUserColorId.set (userColorId + 1)
      callback()

  _initUserChatPresence: ->
    this._initFirebasePresence()
    this._initFirebaseChat()
    this._sendJoinedMessage()

  _initFirebasePresence: ->
    boxId = App.get 'router.boxIdeController.content.id'
    username = App.get 'currentUser.username'
    usercolor = @get 'userColor'
    firebaseUsersURL = "#{App.get 'config.firebaseHost'}/box/#{boxId}/users"
    firebaseUsers = new Firebase firebaseUsersURL
    firebasePresence = firebaseUsers.push {'name': username, 'color': usercolor}
    firebasePresence.onDisconnect().remove()
    @set 'firebasePresence', firebasePresence

  _resetFirebasePresence: ->
    @get('firebasePresence')?.remove()
    @set 'firebasePresence', null

  _initFirebaseChat: ->
    boxId = App.get 'router.boxIdeController.content.id'
    firebaseChatURL = "#{App.get 'config.firebaseHost'}/box/#{boxId}/chat"
    firebaseChat = new Firebase firebaseChatURL
    firebaseChat.endAt().limit(100).on 'child_added', (child) =>
      values = child.val()
      message = App.CollabMessage.create username: values[0], data: values[1], type: values[2]
      @get('messages')?.addObject message
      if message.type == App.CollabMessage.TypeChat
        this._notifyOnChatMention message
    @set 'firebaseChat', firebaseChat

  _resetFirebaseChat: ->
    @get('firebaseChat')?.off()
    @set 'firebaseChat', null

  _notifyOnChatMention: (message) ->
    currentUsername = App.get 'currentUser.username'
    if mentions = this._extractMentions(message.data)
      if mentions.contains(currentUsername)
        editorController = App.get 'router.ideEditorController'
        editorController.set 'chatNotification', message
        sound = new Audio("/sounds/chat_alert.wav")
        sound.play()

  _sendJoinedMessage: ->
    this.sendMessage '', App.CollabMessage.TypeUserJoined

  _sendLeftMessage: ->
    this.sendMessage '', App.CollabMessage.TypeUserLeft
