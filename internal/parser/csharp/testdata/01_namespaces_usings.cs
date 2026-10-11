// SZENARIO: Namensräume (Block- und Datei-Form), verschachtelte Namensräume und alle using-Varianten
// ABDECKUNG: namespace-block, file-scoped namespace, verschachtelter namespace, using, using static, using-Alias, global using
using System;
using System.Collections.Generic;
using static System.Math;
using Alias = System.Text.StringBuilder;
global using System.Linq;

namespace Shop.Core
{
    namespace Inner
    {
        public class Deep { }
    }

    public class Catalog { }
}
