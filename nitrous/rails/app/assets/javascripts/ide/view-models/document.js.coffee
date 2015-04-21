#= require ./tab-content

untitledCount = 0

App.Document = App.TabContent.extend
  isDocumentTab: true

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

  gotoLine: null
  selections: null
  scrollPosition: null

  readOnly: false

  title: (->
    @get('path.name') || "Untitled #{untitledCount += 1}"
  ).property 'path.name'
