App.SharedContainersGroup = Em.Object.extend
  items: []
  userId: null

  username: (->
    @get 'items.firstObject.userUsername'
  ).property 'items.@each'

  name: (->
    @get 'items.firstObject.userName'
  ).property 'items.@each'

  gravatarHash: (->
    @get 'items.firstObject.userGravatarHash'
  ).property 'items.@each.gravatarHash'

  gravatar: (->
    "https://secure.gravatar.com/avatar/#{@get('gravatarHash')}?default=retro&secure=true&size=72"
  ).property 'gravatarHash'
