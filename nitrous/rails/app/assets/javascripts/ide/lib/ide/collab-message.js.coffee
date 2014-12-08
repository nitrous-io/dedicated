App.CollabMessage = klass = Em.Object.extend
  username: null
  type: null
  data: null

  isChatMessage: (->
    type = @get 'type'
    !type || (type == klass.TypeChat)
  ).property 'type'

  hasFileLink: (->
    [klass.TypeFileOpen, klass.TypeCollabMode].contains @get('type')
  ).property 'type'

  message: (->
    switch type = @get 'type'
      when klass.TypeFileOpen
        "opened"
      when klass.TypeCollabMode
        "enabled collab mode on"
      when klass.TypeUserJoined
        "joined the room"
      when klass.TypeUserLeft
        "left the room"
      else
        @get 'data'
  ).property 'type', 'data'

klass.reopenClass
  TypeChat         : 1
  TypeFileOpen     : 2
  TypeCollabMode   : 3
  TypeUserJoined   : 4
  TypeUserLeft     : 5
