#= require ./tab-content

untitledCount = 0

App.Document = App.TabContent.extend
  isDocumentTab: true
  contentType: 'text'

  otClient: null
  otUndoManagerOverride: null

  findTrigger: null
  findNextTrigger: null
  findPrevTrigger: null
  replaceTrigger: null
  replaceAllTrigger: null
  selectNextTrigger: null

  foldCodeTrigger: null
  unfoldCodeTrigger: null
  toggleCommentTrigger: null
  splitSelectionIntoLinesTrigger: null

  goToLine: null
  selections: null
  scrollPosition: null

  readOnly: false

  line: (->
    (@get('selections.0.head.line') || 0) + 1
  ).property 'selections'

  column: (->
    (@get('selections.0.head.ch') || 0) + 1
  ).property 'selections'

  title: (->
    @get('path.name') || "Untitled #{untitledCount += 1}"
  ).property 'path.name'
