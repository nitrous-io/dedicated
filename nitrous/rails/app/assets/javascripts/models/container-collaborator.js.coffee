#= require ./model
attr = SM.attr

App.ContainerCollaborator = klass = App.Model.extend
  id: attr('number', serialize: false)
  containerId: attr('number', as: 'container_id', serialize: false)
  userId: attr('number', as: 'user_id', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'updated_at', serialize: false)
  username: attr('string', as: 'username', serialize: false)
  email: attr('string', as: 'email', serialize: false)
  name: attr('string', as: 'name', serialize: false)

  pendingRemove: attr('boolean', serialize: false)

  gravatar: (->
    "https://secure.gravatar.com/avatar/#{md5(@get 'email')}?default=retro&secure=true&size=48"
  ).property 'email'

  inputId: (->
    if id = @get('id') && cId = @get('containerId')
      "collaborator-#{cId}-#{id}"
  ).property 'id', 'containerId'

klass.reopenClass
  url: '/containers/:containerId/collaborators'
  primaryKey: 'id'
  collectionKey: 'collaborators'
