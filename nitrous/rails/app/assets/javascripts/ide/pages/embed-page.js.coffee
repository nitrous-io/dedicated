window.App ||= {}

class App.EmbedPage
  constructor: ->
    @$htmlCode = $('#html-code')
    @$markdownCode = $('#markdown-code')
    @$htmlCodeTemplate = Handlebars.compile $('#html-code-template').html()
    @$markdownCodeTemplate = Handlebars.compile $('#markdown-code-template').html()
    @$instructions = $('#instructions')

    $('input[name=image-url]').change @generateCode.bind(this)
    $('select[name=runtime]').change @generateCode.bind(this)
    $('input[name=file-to-open]').keydown @generateCode.bind(this)
    $('input[name=repo]').keydown @generateCode.bind(this)

    onClick = -> $(this).select()

    @$htmlCode.click onClick
    @$markdownCode.click onClick

    @generateCode()

  generateCode: ->
    setTimeout(=>
      data = @getData()
      repo = data.repo
      if @validateRepoName(repo)
        @generateHTML(data)
        @generateMarkdown(data)
        @$instructions.html('Copy the following code into your project&rsquo;s README or website.')
      else
        message = if repo
          "#{repo} is not a valid GitHub repository name."
        else
          "GitHub repository name (Step 3) is required."
        @$instructions.html(message)
        @disableTextAreas()
    , 0)

  generateHTML: (data) ->
    output = @$htmlCodeTemplate(data)
    @$htmlCode.val(output).attr('disabled', false)

  generateMarkdown: (data) ->
    output = @$markdownCodeTemplate(data)
    @$markdownCode.val(output).attr('disabled', false)

  disableTextAreas: (text) ->
    @$htmlCode.val('').attr('disabled', true)
    @$markdownCode.val('').attr('disabled', true)

  getData: ->
    e = Handlebars.Utils.escapeExpression
    imageURL: e $('input[name=image-url]:checked').val()
    runtime: e $('select[name=runtime]').val()
    repo: repo = e $('input[name=repo]').val()
    repoEncoded: encodeURIComponent(repo)
    fileToOpen: encodeURIComponent(e $('input[name=file-to-open]').val())

  validateRepoName: (repo) ->
    return unless repo
    repo.match /^([a-zA-Z\d-_]+)\/([a-zA-Z\d-_\.]+)$/
