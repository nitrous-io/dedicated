App.Snackbar = Em.Object.extend
  items: null

  init: ->
    @set 'items', Em.A()

  post: (msg) ->
    items = @get('items')
    item = items.unshiftObject(App.SnackbarItem.create(message: msg.toString()))

    # fade in
    window.setTimeout(->
      item.set 'fadein', true
    , 100)
    # fade out
    window.setTimeout(->
      item.set 'fadeout', true
    , 5500)
    # and then kill
    window.setTimeout(->
      items.removeObject(item)
    , 6500)

App.SnackbarItem = Em.Object.extend
  message: null
  fadein: false
  fadeout: false
