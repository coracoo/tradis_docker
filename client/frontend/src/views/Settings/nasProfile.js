export function createDeploymentPolicyForm(profile = {}) {
  const libraryPaths = profile.libraryPaths || {}
  const networkPolicy = profile.networkPolicy || {}
  return {
    puid: Number.isInteger(profile.puid) ? profile.puid : 1000,
    pgid: Number.isInteger(profile.pgid) ? profile.pgid : 1000,
    timezone: profile.timezone || 'Asia/Shanghai',
    allowHost: networkPolicy.allowHost !== false,
    mediaPath: libraryPaths.media || '',
    novelPath: libraryPaths.novel || '',
    comicPath: libraryPaths.comic || '',
    musicPath: libraryPaths.music || '',
    photoPath: libraryPaths.photo || ''
  }
}

export function buildDeploymentPolicyUpdate(form = {}) {
  return {
    puid: Number(form.puid),
    pgid: Number(form.pgid),
    timezone: String(form.timezone || '').trim() || 'Asia/Shanghai',
    networkPolicy: { allowHost: form.allowHost !== false },
    libraryPaths: {
      media: String(form.mediaPath || '').trim(),
      novel: String(form.novelPath || '').trim(),
      comic: String(form.comicPath || '').trim(),
      music: String(form.musicPath || '').trim(),
      photo: String(form.photoPath || '').trim()
    }
  }
}
