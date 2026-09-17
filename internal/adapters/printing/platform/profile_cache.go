package platform

import (
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// ProfileNormalizer transforma un perfil en su versión ejecutable en esta
// plataforma. En Windows aplica NormalizeForWindows con el modo de la impresora;
// en Linux/mac no hay normalizador. Se pasa como función para poder testear el
// wrapper sin depender del SO donde corre el test.
type ProfileNormalizer func(dp.PrinterProfile) dp.PrinterProfile

// NormalizingProfileCache envuelve un ProfileCache aplicando el normalizador a
// cada perfil que SALE por Profile(). Se guarda el perfil tal como lo envió el
// Backend y se traduce al leerlo.
//
// Se normaliza al leer, y no al guardar, porque la traducción depende de
// configuración que el panel cambia en caliente (el modo vectorial/imagen de
// cada impresora). Normalizando al guardar, cambiar el modo obligaría a olvidar
// el perfil, y un perfil del Backend olvidado no vuelve hasta la siguiente
// sincronización: la impresora se quedaría sin perfil entretanto. El coste es
// una copia de un struct pequeño por trabajo.
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
	p, err := c.inner.Profile(id)
	if err != nil {
		return p, err
	}
	return c.normalize(p), nil
}

func (c *normalizingCache) Set(p dp.PrinterProfile)       { c.inner.Set(p) }
func (c *normalizingCache) SetAll(ps []dp.PrinterProfile) { c.inner.SetAll(ps) }
func (c *normalizingCache) Forget(id string)              { c.inner.Forget(id) }
