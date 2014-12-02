App.IdePathContextMenuView = Em.View.extend
  templateName: 'templates/box/ide/path-context-menu'
  elementId: 'ide_path_context_menu_view'
  classNames: ['dropdown']

  path: null

  willDestroyElement: ->
    @_super()
    @set 'path', null

  show: (event, path) ->
    @set 'path', path
    offset = 5
    top = event.clientY - offset
    left = event.clientX - offset

    @$('.dropdown-menu').css
      top: if top > 0 then top else 0
      left: if left > 0 then left else 0

    @$().addClass 'open'

  hide: ->
    @$().removeClass 'open'

  didClickOpen: ->
    path = @get 'path'
    @get('controller').openPath path

  didClickRefresh: ->
    path = @get 'path'
    @get('controller').refresh path

  didClickFindInFolder: ->
    path = @get 'path'
    @get('controller').findInFolder path

  didClickNewFile: ->
    path = @get 'path'
    @get('controller').newPath path, false

  didClickNewFolder: ->
    path = @get 'path'
    @get('controller').newPath path, true

  didClickUploadFiles: ->
    path = @get 'path'
    @get('controller').showUploadDialog path

  didClickSetFolderAsRoot: ->
    path = @get 'path'
    @get('controller').setPathAsRoot path

  didClickRename: ->
    path = @get 'path'
    @get('controller').renamePath path

  didClickDelete: ->
    path = @get 'path'
    @get('controller').confirmDelete path

  didClickDownloadFile: ->
    path = @get 'path'
    @get('controller').downloadPath path
