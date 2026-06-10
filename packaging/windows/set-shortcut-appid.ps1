param(
    [Parameter(Mandatory = $true)]
    [string] $ShortcutPath,

    [Parameter(Mandatory = $true)]
    [string] $AppUserModelID
)

$ErrorActionPreference = 'Stop'

$source = @'
using System;
using System.Runtime.InteropServices;

namespace NtfyInstaller
{
    [ComImport]
    [Guid("00021401-0000-0000-C000-000000000046")]
    public class ShellLink
    {
    }

    [ComImport]
    [InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
    [Guid("0000010b-0000-0000-C000-000000000046")]
    public interface IPersistFile
    {
        void GetClassID(out Guid pClassID);
        void IsDirty();
        void Load([MarshalAs(UnmanagedType.LPWStr)] string pszFileName, uint dwMode);
        void Save([MarshalAs(UnmanagedType.LPWStr)] string pszFileName, bool fRemember);
        void SaveCompleted([MarshalAs(UnmanagedType.LPWStr)] string pszFileName);
        void GetCurFile([MarshalAs(UnmanagedType.LPWStr)] out string ppszFileName);
    }

    [StructLayout(LayoutKind.Sequential, Pack = 4)]
    public struct PropertyKey
    {
        public Guid fmtid;
        public uint pid;

        public PropertyKey(Guid fmtid, uint pid)
        {
            this.fmtid = fmtid;
            this.pid = pid;
        }
    }

    [StructLayout(LayoutKind.Sequential)]
    public struct PropVariant
    {
        public ushort vt;
        public ushort wReserved1;
        public ushort wReserved2;
        public ushort wReserved3;
        public IntPtr p;
        public int p2;

        public static PropVariant FromString(string value)
        {
            return new PropVariant
            {
                vt = 31,
                p = Marshal.StringToCoTaskMemUni(value)
            };
        }
    }

    [ComImport]
    [InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
    [Guid("886D8EEB-8CF2-4446-8D02-CDBA1DBDCF99")]
    public interface IPropertyStore
    {
        [PreserveSig]
        int GetCount(out uint cProps);
        [PreserveSig]
        int GetAt(uint iProp, out PropertyKey pkey);
        [PreserveSig]
        int GetValue(ref PropertyKey key, out PropVariant pv);
        [PreserveSig]
        int SetValue(ref PropertyKey key, ref PropVariant propvar);
        [PreserveSig]
        int Commit();
    }

    public static class ShortcutAppID
    {
        private static readonly PropertyKey AppUserModelID = new PropertyKey(
            new Guid("9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3"),
            5);

        [DllImport("Ole32.dll")]
        private static extern int PropVariantClear(ref PropVariant pvar);

        public static void Set(string shortcutPath, string appUserModelID)
        {
            object shellLink = new ShellLink();
            var persistFile = (IPersistFile)shellLink;
            persistFile.Load(shortcutPath, 2);

            var propertyStore = (IPropertyStore)shellLink;
            PropVariant value = PropVariant.FromString(appUserModelID);
            try
            {
                PropertyKey appUserModelIDKey = AppUserModelID;
                Marshal.ThrowExceptionForHR(propertyStore.SetValue(ref appUserModelIDKey, ref value));
                Marshal.ThrowExceptionForHR(propertyStore.Commit());
                persistFile.Save(shortcutPath, true);
            }
            finally
            {
                PropVariantClear(ref value);
                Marshal.ReleaseComObject(shellLink);
            }
        }
    }
}
'@

Add-Type -TypeDefinition $source
[NtfyInstaller.ShortcutAppID]::Set($ShortcutPath, $AppUserModelID)
