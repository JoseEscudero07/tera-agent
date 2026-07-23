package platform

import (
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// ProfileNormalizer transforma un perfil entrante en su versión ejecutable en
// esta plataforma. En Windows aplica NormalizeForGDIRaster; en Linux/mac es
// identidad. Se pasa como función para poder testear el wrapper sin depender
// del SO donde corre el test.
type ProfileNormalizer func(dp.PrinterProfile) dp.PrinterProfile

// NormalizingProfileCache envuelve un ProfileCache aplicando el normalizador
// a todo perfil que entra por Set/SetAll. Los perfiles quedan almacenados ya
// traducidos, así que Profile() devuelve directamente lo que este SO sabe
// manejar: la traducción DevicePDF→DeviceGDIRaster en Windows ocurre una sola
// vez, no en cada consulta.
//
// El decorador es transparente a los llamantes; ni el lifecycle ni la UI ni
// el print engine saben que existe. Si normalize es nil se comporta como el
// cache pelado.
func NormalizingProfileCache(inner dp.ProfileCache, normalize ProfileNormalizer) dp.ProfileCache {
	if normalize == nil {
		return inner
	}
	return &normalizingCache{inner: inner, normalize: normalize}
}

type normalizingCache struct {
	inner     dp.ProfileCache
	normalize ProfileNormalizer
}

func (c *normalizingCache) Profile(id string) (dp.PrinterProfile, error) {
	return c.inner.Profile(id)
}

func (c *normalizingCache) Set(p dp.PrinterProfile) { c.inner.Set(c.normalize(p)) }
func (c *normalizingCache) Forget(id string)        { c.inner.Forget(id) }
func (c *normalizingCache) SetAll(ps []dp.PrinterProfile) {
	out := make([]dp.PrinterProfile, len(ps))
	for i, p := range ps {
		out[i] = c.normalize(p)
	}
	c.inner.SetAll(out)
}
