<%-- SZENARIO: ASPX-Seite mit Direktiven, Code-Block, Ausdruck und eingebettetem Script mit Methode
     ABDECKUNG: <%@ Page Inherits, <%@ Import Namespace, <%= Ausdruck, <% Schleife mit HTML, Kommentar, <script runat="server"> mit Methode --%>
<%@ Page Language="C#" Inherits="Shop.BasePage" %>
<%@ Import Namespace="Shop.Models" %>
<html>
<body>
<h1><%= Title %></h1>
<% foreach (var o in Orders) { %>
    <li><%= Format(o.Total) %></li>
<% } %>
</body>
</html>
<script runat="server">
    private string Format(decimal value) { return value.ToString("C"); }
</script>
