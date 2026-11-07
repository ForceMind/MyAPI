// Synthetic Node child only. It opens no socket, database, window or provider.
if (!process.send || !process.env.MYAPI_DESKTOP_CHILD_FIXTURE) {
  throw new Error('This fixture requires an owned Actions IPC subprocess')
}
process.on('message', (message) => {
  if (message === 'exit') process.exit(0)
})
process.send({ ready: true })
