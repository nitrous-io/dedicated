window.App.reopen
  postError: (err={}, def) -> # def: default
    if typeof err == 'string'
      msg = err?.capitalize()
    msg ||= err.error_description?.capitalize()
    unless msg
      msg = def?.capitalize() || "An unexpected error has occurred. If you require assistance, please contact us at support@nitrous.io"
    App.snackbar.post(msg, sticky: true)
