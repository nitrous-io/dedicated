App.IdeManageAutopartsDialogView = App.IdeModalDialogView.extend
  templateName: 'templates/box/ide/manage-autoparts-dialog'
  elementId: 'ide_manage_autoparts_dialog_view'
  contentBinding: 'controller.manageDialog' # controller = ideAutopartsController

  loading: true
  hidden: true
  processingView: false

  parts: null
  visibleParts: null

  filter: null
  loadParts: null
  searchTerm: null
  showInstalled: false

  partsPerPage: 4
  currentPage: 0

  totalPages: (->
    if parts = @get('parts')
      partsCount = parts.get 'length'
      partsPerPage = @get 'partsPerPage'
      Math.ceil (partsCount / partsPerPage)
  ).property 'parts'

  viewDidAppear: ->
    @_super()
    @set 'hidden', false

  viewDidDisappear: ->
    @set 'hidden', true
    @set 'searchTerm', null
    @set 'currentPage', 0 # reset back to first page

  hidePager: (->
    @get('loading') || @get('controller.processing') || @get('totalPages') < 2
  ).property 'loading', 'totalPages', 'controller.processing'

  inFirstPage: (->
    @get('currentPage') == 1
  ).property 'currentPage'

  inLastPage: (->
    @get('currentPage') == @get('totalPages')
  ).property 'currentPage', 'totalPages'

  processingView: (->
    @get('controller.processing') || null
  ).property 'controller.processing'

  showDoneButton: (->
    @get('processingView') && @get('controller.partsSocket.outputStream.completed')
  ).property 'controller.partsSocket.outputStream.completed', 'processingView'

  didClickNext: ->
    @incrementProperty 'currentPage'

  didClickPrevious: ->
    @decrementProperty 'currentPage'

  didClickDone: ->
    if controller = @get 'controller'
      controller.set 'processing', false
      controller.set 'processingPart', null

  # Try loading parts, only when dialog becomes visible and
  # if controller itself has finished loading the parts.
  _searchForParts: (->
    return if @get 'hidden'

    @set 'loading', true
    @set 'currentPage', 0
    @get('controller').search @get('searchTerm'), @get('showInstalled'), (matchingParts) =>
      @set 'parts', matchingParts
      @set 'loading', false
      @set 'currentPage', 1
  ).observes 'controller.parts', 'hidden', 'searchTerm', 'showInstalled'

  # when returning back from process view,
  # re-search the updated parts list
  # and correctly filter installed/uninstalled parts
  _didReturnFromProcessingView: (->
    return if @get 'processingView'
    @get('controller').search @get('searchTerm'), @get('showInstalled'), (matchingParts) =>
      @set 'parts', matchingParts
  ).observes 'processingView'

  _setVisibleParts: (->
    return unless parts = @get('parts')
    if (currentPage = @get 'currentPage') > 0
      partsPerPage = @get 'partsPerPage'
      partsStart = (currentPage - 1) * partsPerPage
      partsEnd = partsStart + partsPerPage
      visibleParts = parts.slice(partsStart, partsEnd)

      @set 'visibleParts', visibleParts

      # If a page lists only one part,
      # and if it's uninstalled,
      # list should be set to the previous page.
      unless visibleParts.length
        @decrementProperty 'currentPage'
  ).observes 'currentPage', 'parts'
