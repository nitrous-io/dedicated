App.Snackbar = Em.Object.extend
  items: null

  init: ->
    @set 'items', Em.A()

  containsMessage: (msg) ->
    items = @get('items')
    items.any (item)->
      item.message == msg

  post: (msg, duration=5500) ->
    return if @containsMessage msg
    items = @get('items')
    item = items.unshiftObject(App.SnackbarItem.create(message: msg.toString()))

    # fade in
    window.setTimeout(->
      item.set 'fadein', true
    , 100)
    # fade out
    window.setTimeout(->
      item.set 'fadeout', true
    , duration)
    # and then kill
    window.setTimeout(->
      items.removeObject(item)
    , duration+1000)

App.SnackbarItem = Em.Object.extend
  message: null
  fadein: false
  fadeout: false
